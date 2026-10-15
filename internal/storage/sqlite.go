package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/MIK-HEAL/TraceAI/internal/events"
)

type SQLiteStorage struct {
	mu      sync.RWMutex
	path    string
	db      *sql.DB
	options SQLiteOptions
}

type CaptureMode string

const (
	MetadataOnly CaptureMode = "metadata-only"
	Redacted     CaptureMode = "redacted"
	FullPayload  CaptureMode = "full-payload"
)

type SQLiteOptions struct {
	RetentionDays int
	MaxBytes      int64
	CaptureMode   CaptureMode
	RedactKeys    []string
}

func NewSQLiteStorage(path string) *SQLiteStorage {
	if path == "" {
		path = "traceai.db"
	}
	return &SQLiteStorage{path: path, options: SQLiteOptions{CaptureMode: Redacted}}
}

func NewSQLiteStorageWithOptions(path string, options SQLiteOptions) *SQLiteStorage {
	if path == "" {
		path = "traceai.db"
	}
	if options.CaptureMode == "" {
		options.CaptureMode = Redacted
	}
	return &SQLiteStorage{path: path, options: options}
}

func (s *SQLiteStorage) Init(ctx context.Context) error {
	logger := slog.Default().With("component", "storage", "backend", "sqlite")
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db != nil {
		return fmt.Errorf("storage already initialized")
	}

	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		logger.Error("storage init failed", "error", err)
		return err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=ON",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			_ = db.Close()
			logger.Error("storage configuration failed", "pragma", pragma, "error", err)
			return fmt.Errorf("configure sqlite: %w", err)
		}
	}
	if err := s.migrate(ctx, db); err != nil {
		_ = db.Close()
		logger.Error("storage migration failed", "error", err)
		return err
	}
	if err := redactLegacyMCPCommands(ctx, db); err != nil {
		_ = db.Close()
		logger.Error("storage metadata migration failed", "error", err)
		return err
	}
	s.db = db
	logger.Info("storage initialized", "path", s.path)
	return nil
}

func (s *SQLiteStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	if err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("storage close failed", "error", err)
		return err
	}
	s.db = nil
	slog.Default().With("component", "storage", "backend", "sqlite").Info("storage closed")
	return nil
}

func (s *SQLiteStorage) Ping(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	db := s.db
	if db == nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Warn("storage ping failed", "reason", "not_initialized")
		return fmt.Errorf("storage not initialized")
	}
	return db.PingContext(ctx)
}

func (s *SQLiteStorage) UpsertToolCatalog(ctx context.Context, sessionID, serverName string, tools []CatalogTool) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return fmt.Errorf("storage not initialized")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, tool := range tools {
		if strings.TrimSpace(tool.Name) == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_tool_catalog (server_name, tool_name, description, first_seen, last_seen, session_id) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?) ON CONFLICT(server_name, tool_name) DO UPDATE SET description=excluded.description, last_seen=excluded.last_seen, session_id=excluded.session_id`, serverName, tool.Name, tool.Description, sessionID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStorage) ToolCatalog(ctx context.Context, serverName string) ([]CatalogTool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT tool_name, description FROM mcp_tool_catalog WHERE server_name = ? ORDER BY tool_name`, serverName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CatalogTool
	for rows.Next() {
		var item CatalogTool
		if err := rows.Scan(&item.Name, &item.Description); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) ToolUsage(ctx context.Context, since time.Time, limit int) ([]ToolUsage, error) {
	tmp := &MemoryStorage{}
	if err := s.copyInto(ctx, tmp, since); err != nil {
		return nil, err
	}
	s.mu.RLock()
	rows, err := s.db.QueryContext(ctx, `SELECT server_name, tool_name, description FROM mcp_tool_catalog`)
	s.mu.RUnlock()
	if err == nil {
		for rows.Next() {
			var server, name, description string
			if scanErr := rows.Scan(&server, &name, &description); scanErr == nil {
				_ = tmp.UpsertToolCatalog(ctx, "", server, []CatalogTool{{Name: name, Description: description}})
			}
		}
		_ = rows.Close()
	}
	return tmp.ToolUsage(ctx, since, limit)
}

func (s *SQLiteStorage) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	event = event.Normalize()
	if err := event.Validate(); err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}
	metadata, err := marshalMetadata(sanitizeMetadataWithOptions(event.Metadata, s.options))
	if err != nil {
		return fmt.Errorf("marshal event metadata: %w", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	db := s.db
	if db == nil {
		err := fmt.Errorf("storage not initialized")
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (
			event_id, schema_version, trace_id, session_id, request_id, call_id, parent_event_id, attempt, timestamp,
			agent_name, agent_version, adapter_name, adapter_version,
			tool_type, tool_name, function_name, success, duration_ms,
			input_size, output_size, retry_count, error_type, error_code, error_message, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.EventID, event.SchemaVersion, event.TraceID, event.SessionID, event.RequestID, event.CallID, event.ParentEventID, event.Attempt, event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.AgentName, event.AgentVersion, event.AdapterName, event.AdapterVersion,
		event.ToolType, event.ToolName, event.FunctionName, boolToInt(event.Success), event.DurationMS,
		event.InputSize, event.OutputSize, event.RetryCount, event.ErrorType, event.ErrorCode, event.ErrorMessage, metadata); err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (session_id, agent_name, adapter_name, first_seen, last_seen, call_count)
		VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT(session_id) DO UPDATE SET
			last_seen = excluded.last_seen,
			call_count = call_count + 1
	`, event.SessionID, event.AgentName, event.AdapterName, event.Timestamp.UTC().Format(time.RFC3339Nano), event.Timestamp.UTC().Format(time.RFC3339Nano)); err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO agents (agent_name, agent_version, call_count, success_count, total_duration_ms, input_size, output_size, last_seen)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_name) DO UPDATE SET
			agent_version = excluded.agent_version,
			call_count = call_count + 1,
			success_count = success_count + excluded.success_count,
			total_duration_ms = total_duration_ms + excluded.total_duration_ms,
			input_size = input_size + excluded.input_size,
			output_size = output_size + excluded.output_size,
			last_seen = excluded.last_seen
	`, event.AgentName, event.AgentVersion, boolToInt(event.Success), event.DurationMS, event.InputSize, event.OutputSize, event.Timestamp.UTC().Format(time.RFC3339Nano)); err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tools (tool_name, tool_type, call_count, success_count, total_duration_ms, input_size, output_size, last_seen)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?)
		ON CONFLICT(tool_name) DO UPDATE SET
			tool_type = excluded.tool_type,
			call_count = call_count + 1,
			success_count = success_count + excluded.success_count,
			total_duration_ms = total_duration_ms + excluded.total_duration_ms,
			input_size = input_size + excluded.input_size,
			output_size = output_size + excluded.output_size,
			last_seen = excluded.last_seen
	`, event.ToolName, event.ToolType, boolToInt(event.Success), event.DurationMS, event.InputSize, event.OutputSize, event.Timestamp.UTC().Format(time.RFC3339Nano)); err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}

	day := event.Timestamp.UTC().Format("2006-01-02")
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO daily_stats (stat_day, call_count, success_count, total_duration_ms, input_size, output_size)
		VALUES (?, 1, ?, ?, ?, ?)
		ON CONFLICT(stat_day) DO UPDATE SET
			call_count = call_count + 1,
			success_count = success_count + excluded.success_count,
			total_duration_ms = total_duration_ms + excluded.total_duration_ms,
			input_size = input_size + excluded.input_size,
			output_size = output_size + excluded.output_size
	`, day, boolToInt(event.Success), event.DurationMS, event.InputSize, event.OutputSize); err != nil {
		slog.Default().With("component", "storage", "backend", "sqlite").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStorage) ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error) {
	return s.listEvents(ctx, time.Time{}, limit)
}

func (s *SQLiteStorage) listEvents(ctx context.Context, since time.Time, limit int) ([]events.ToolEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	where, args := sinceClause(since)
	limitClause := ""
	if limit > 0 {
		limitClause = "LIMIT ?"
		args = append(args, limit)
	}
	query := fmt.Sprintf(`
		SELECT event_id, schema_version, trace_id, session_id, request_id, call_id, parent_event_id, attempt, timestamp,
			agent_name, agent_version, adapter_name, adapter_version,
			tool_type, tool_name, function_name, success, duration_ms,
			input_size, output_size, retry_count, error_type, error_code, error_message, metadata
		FROM events
		%s
		ORDER BY timestamp DESC
		%s
	`, where, limitClause)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []events.ToolEvent
	for rows.Next() {
		var event events.ToolEvent
		var timestamp string
		var metadata string
		var success int
		if err := rows.Scan(
			&event.EventID, &event.SchemaVersion, &event.TraceID, &event.SessionID, &event.RequestID, &event.CallID, &event.ParentEventID, &event.Attempt, &timestamp,
			&event.AgentName, &event.AgentVersion, &event.AdapterName, &event.AdapterVersion,
			&event.ToolType, &event.ToolName, &event.FunctionName, &success, &event.DurationMS,
			&event.InputSize, &event.OutputSize, &event.RetryCount, &event.ErrorType, &event.ErrorCode, &event.ErrorMessage, &metadata,
		); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, timestamp)
		if err != nil {
			return nil, fmt.Errorf("parse event %q timestamp: %w", event.EventID, err)
		}
		event.Timestamp = parsed
		event.Success = success == 1
		event.Metadata, err = unmarshalMetadata(metadata)
		if err != nil {
			return nil, fmt.Errorf("parse event %q metadata: %w", event.EventID, err)
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) TopTools(ctx context.Context, since time.Time, limit int) ([]ToolCount, error) {
	return s.topCounts(ctx, since, limit, `
		SELECT tool_name AS name, COUNT(*) AS calls, COALESCE(SUM(success), 0) AS success
		FROM events
	`, `tool_name`)
}

func (s *SQLiteStorage) TopFunctions(ctx context.Context, since time.Time, limit int) ([]FunctionCount, error) {
	items, err := s.topCounts(ctx, since, limit, `
		SELECT function_name AS name, COUNT(*) AS calls, COALESCE(SUM(success), 0) AS success
		FROM events
	`, `function_name`)
	if err != nil {
		return nil, err
	}
	out := make([]FunctionCount, len(items))
	for i, item := range items {
		out[i] = FunctionCount{FunctionName: item.ToolName, Calls: item.Calls, Success: item.Success}
	}
	return out, nil
}

func (s *SQLiteStorage) TopAgents(ctx context.Context, since time.Time, limit int) ([]AgentCount, error) {
	items, err := s.topCounts(ctx, since, limit, `
		SELECT agent_name AS name, COUNT(*) AS calls, COALESCE(SUM(success), 0) AS success
		FROM events
	`, `agent_name`)
	if err != nil {
		return nil, err
	}
	out := make([]AgentCount, len(items))
	for i, item := range items {
		out[i] = AgentCount{AgentName: item.ToolName, Calls: item.Calls, Success: item.Success}
	}
	return out, nil
}

func (s *SQLiteStorage) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]ToolFailureRate, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	where, args := sinceClause(since)
	limitClause := ""
	if limit > 0 {
		limitClause = "LIMIT ?"
		args = append(args, limit)
	}
	query := fmt.Sprintf(`
		SELECT tool_name,
			COUNT(*) AS calls,
			COUNT(*) - COALESCE(SUM(success), 0) AS failures,
			CASE WHEN COUNT(*) = 0 THEN 0 ELSE CAST(COUNT(*) - COALESCE(SUM(success), 0) AS REAL) / COUNT(*) END AS failure_rate
		FROM events %s
		GROUP BY tool_name
		ORDER BY failure_rate DESC, calls DESC, tool_name ASC
		%s
	`, where, limitClause)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolFailureRate
	for rows.Next() {
		var item ToolFailureRate
		if err := rows.Scan(&item.ToolName, &item.Calls, &item.Failures, &item.FailureRate); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) Stats(ctx context.Context, since time.Time) (Stats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return Stats{}, fmt.Errorf("storage not initialized")
	}
	where, args := sinceClause(since)
	query := fmt.Sprintf(`
		SELECT COUNT(*) AS calls,
			COALESCE(SUM(success), 0) AS success_count,
			COALESCE(AVG(duration_ms), 0) AS avg_latency_ms,
			COALESCE(SUM(input_size), 0) AS input_size,
			COALESCE(SUM(output_size), 0) AS output_size
		FROM events %s
	`, where)
	row := s.db.QueryRowContext(ctx, query, args...)
	var calls, successCount, inputSize, outputSize int64
	var avgLatency float64
	if err := row.Scan(&calls, &successCount, &avgLatency, &inputSize, &outputSize); err != nil {
		return Stats{}, err
	}
	stats := Stats{Calls: calls, InputSize: inputSize, OutputSize: outputSize, AvgLatency: avgLatency}
	if calls > 0 {
		stats.SuccessRate = float64(successCount) / float64(calls)
	}
	return stats, nil
}

func (s *SQLiteStorage) DailyStats(ctx context.Context, since time.Time) ([]DailyStat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	where, args := sinceDayClause(since)
	query := fmt.Sprintf(`
		SELECT stat_day, call_count, success_count, total_duration_ms, input_size, output_size
		FROM daily_stats %s
		ORDER BY stat_day ASC
	`, where)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DailyStat
	for rows.Next() {
		var item DailyStat
		if err := rows.Scan(&item.StatDay, &item.Calls, &item.Success, &item.TotalDurationMS, &item.InputSize, &item.OutputSize); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) MonthlyStats(ctx context.Context, since time.Time) ([]MonthlyStat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	where, args := sinceClause(since)
	query := fmt.Sprintf(`
		SELECT substr(timestamp, 1, 7) AS stat_month,
			COUNT(*) AS calls,
			COALESCE(SUM(success), 0) AS success_count,
			COALESCE(SUM(duration_ms), 0) AS total_duration_ms,
			COALESCE(SUM(input_size), 0) AS input_size,
			COALESCE(SUM(output_size), 0) AS output_size
		FROM events %s
		GROUP BY stat_month
		ORDER BY stat_month ASC
	`, where)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []MonthlyStat
	for rows.Next() {
		var item MonthlyStat
		if err := rows.Scan(&item.StatMonth, &item.Calls, &item.Success, &item.TotalDurationMS, &item.InputSize, &item.OutputSize); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) WeeklyStats(ctx context.Context, since time.Time) ([]WeeklyStat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	where, args := sinceClause(since)
	query := fmt.Sprintf(`
		SELECT strftime('%%Y-W%%W', timestamp) AS stat_week,
			COUNT(*) AS calls,
			COALESCE(SUM(success), 0) AS success_count,
			COALESCE(SUM(duration_ms), 0) AS total_duration_ms,
			COALESCE(SUM(input_size), 0) AS input_size,
			COALESCE(SUM(output_size), 0) AS output_size
		FROM events %s
		GROUP BY stat_week
		ORDER BY stat_week ASC
	`, where)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []WeeklyStat
	for rows.Next() {
		var item WeeklyStat
		if err := rows.Scan(&item.StatWeek, &item.Calls, &item.Success, &item.TotalDurationMS, &item.InputSize, &item.OutputSize); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]ErrorBreakdown, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	where, args := sinceClause(since)
	limitClause := ""
	if limit > 0 {
		limitClause = "LIMIT ?"
		args = append(args, limit)
	}
	failureClause := "(success = 0 OR error_type != '' OR error_code != '' OR error_message != '')"
	combinedWhere := "WHERE " + failureClause
	if where != "" {
		combinedWhere = where + " AND " + failureClause
	}
	query := fmt.Sprintf(`
		SELECT error_type, error_code,
			COUNT(*) AS calls,
			COUNT(*) - COALESCE(SUM(success), 0) AS failures
		FROM events %s
		GROUP BY error_type, error_code
		ORDER BY failures DESC, calls DESC, error_type ASC, error_code ASC
		%s
	`, combinedWhere, limitClause)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ErrorBreakdown
	for rows.Next() {
		var item ErrorBreakdown
		if err := rows.Scan(&item.ErrorType, &item.ErrorCode, &item.Calls, &item.Failures); err != nil {
			return nil, err
		}
		item.Category = classifyFailure(item.ErrorType, item.ErrorCode, "")
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) topCounts(ctx context.Context, since time.Time, limit int, baseQuery, column string) ([]ToolCount, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	switch column {
	case "tool_name", "function_name", "agent_name":
	default:
		return nil, fmt.Errorf("unsupported aggregation column %q", column)
	}
	where, args := sinceClause(since)
	limitClause := ""
	if limit > 0 {
		limitClause = "LIMIT ?"
		args = append(args, limit)
	}
	query := fmt.Sprintf(`
		%s %s
		GROUP BY %s
		ORDER BY calls DESC, name ASC
		%s
	`, baseQuery, where, column, limitClause)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ToolCount
	for rows.Next() {
		var item ToolCount
		if err := rows.Scan(&item.ToolName, &item.Calls, &item.Success); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStorage) migrate(ctx context.Context, db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS events (
			event_id TEXT PRIMARY KEY,
			schema_version TEXT NOT NULL,
			trace_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			request_id TEXT NOT NULL DEFAULT '',
			call_id TEXT NOT NULL DEFAULT '',
			parent_event_id TEXT NOT NULL DEFAULT '',
			attempt INTEGER NOT NULL DEFAULT 0,
			timestamp TEXT NOT NULL,
			agent_name TEXT NOT NULL,
			agent_version TEXT NOT NULL,
			adapter_name TEXT NOT NULL,
			adapter_version TEXT NOT NULL,
			tool_type TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			function_name TEXT NOT NULL,
			success INTEGER NOT NULL,
			duration_ms INTEGER NOT NULL,
			input_size INTEGER NOT NULL,
			output_size INTEGER NOT NULL,
			retry_count INTEGER NOT NULL,
			error_type TEXT NOT NULL DEFAULT '',
			error_code TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			metadata TEXT NOT NULL DEFAULT '{}'
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			session_id TEXT PRIMARY KEY,
			agent_name TEXT NOT NULL,
			adapter_name TEXT NOT NULL,
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			call_count INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS agents (
			agent_name TEXT PRIMARY KEY,
			agent_version TEXT NOT NULL,
			call_count INTEGER NOT NULL,
			success_count INTEGER NOT NULL,
			total_duration_ms INTEGER NOT NULL,
			input_size INTEGER NOT NULL,
			output_size INTEGER NOT NULL,
			last_seen TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tools (
			tool_name TEXT PRIMARY KEY,
			tool_type TEXT NOT NULL,
			call_count INTEGER NOT NULL,
			success_count INTEGER NOT NULL,
			total_duration_ms INTEGER NOT NULL,
			input_size INTEGER NOT NULL,
			output_size INTEGER NOT NULL,
			last_seen TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS daily_stats (
			stat_day TEXT PRIMARY KEY,
			call_count INTEGER NOT NULL,
			success_count INTEGER NOT NULL,
			total_duration_ms INTEGER NOT NULL,
			input_size INTEGER NOT NULL,
			output_size INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS mcp_tool_catalog (
			server_name TEXT NOT NULL,
			tool_name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL,
			session_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (server_name, tool_name)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_events_session_timestamp ON events(session_id, timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_events_tool_name ON events(tool_name);`,
		`CREATE INDEX IF NOT EXISTS idx_events_agent_name ON events(agent_name);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	// Add columns to databases created by schema v1. SQLite has no portable
	// IF NOT EXISTS form for ADD COLUMN, so duplicate-column errors are safe.
	for _, stmt := range []string{
		`ALTER TABLE events ADD COLUMN request_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN call_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN parent_event_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE events ADD COLUMN attempt INTEGER NOT NULL DEFAULT 0`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return err
		}
	}
	return nil
}

// redactLegacyMCPCommands removes command lines saved by older versions. A
// command's arguments can carry bearer tokens, API keys, or credentials and
// should never be persisted in a telemetry database.
func redactLegacyMCPCommands(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, `SELECT event_id, metadata FROM events WHERE metadata LIKE ?`, "%mcp_server_cmd%")
	if err != nil {
		return err
	}
	type update struct {
		eventID  string
		metadata string
	}
	var updates []update
	for rows.Next() {
		var eventID, raw string
		if err := rows.Scan(&eventID, &raw); err != nil {
			_ = rows.Close()
			return err
		}
		metadata, err := unmarshalMetadata(raw)
		if err != nil {
			slog.Default().With("component", "storage", "backend", "sqlite", "event_id", eventID).Warn("skipped malformed legacy metadata", "error", err)
			continue
		}
		if _, ok := metadata["mcp_server_cmd"]; !ok {
			continue
		}
		delete(metadata, "mcp_server_cmd")
		encoded, err := marshalMetadata(metadata)
		if err != nil {
			_ = rows.Close()
			return err
		}
		updates = append(updates, update{eventID: eventID, metadata: encoded})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(updates) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, item := range updates {
		if _, err := tx.ExecContext(ctx, `UPDATE events SET metadata = ? WHERE event_id = ?`, item.metadata, item.eventID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func sinceClause(since time.Time) (string, []any) {
	if since.IsZero() {
		return "", nil
	}
	return "WHERE timestamp >= ?", []any{since.UTC().Format(time.RFC3339Nano)}
}

func sinceDayClause(since time.Time) (string, []any) {
	if since.IsZero() {
		return "", nil
	}
	return "WHERE stat_day >= ?", []any{since.UTC().Format("2006-01-02")}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Call sequence and retry pattern analysis (M203)
// Implemented by loading events and delegating to the same in-memory analysis
// used by MemoryStorage.  A future optimisation could push this into SQL
// window functions.
// ---------------------------------------------------------------------------

func (s *SQLiteStorage) CallSequences(ctx context.Context, since time.Time, depth, limit int) ([]CallSequence, error) {
	tmp := &MemoryStorage{}
	if err := s.copyInto(ctx, tmp, since); err != nil {
		return nil, err
	}
	return tmp.CallSequences(ctx, since, depth, limit)
}

func (s *SQLiteStorage) RetryPatterns(ctx context.Context, since time.Time, limit int) ([]RetryPattern, error) {
	tmp := &MemoryStorage{}
	if err := s.copyInto(ctx, tmp, since); err != nil {
		return nil, err
	}
	return tmp.RetryPatterns(ctx, since, limit)
}

// copyInto copies events from SQLite into a MemoryStorage.
func (s *SQLiteStorage) copyInto(ctx context.Context, dest *MemoryStorage, since time.Time) error {
	events, err := s.listEvents(ctx, since, 0)
	if err != nil {
		return err
	}
	for _, e := range events {
		if err := dest.InsertEvent(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

func marshalMetadata(v map[string]interface{}) (string, error) {
	if len(v) == 0 {
		return "{}", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// sanitizeMetadata prevents legacy callers from reintroducing the raw MCP
// command-line field that older proxy versions persisted.
func sanitizeMetadata(v map[string]interface{}) map[string]interface{} {
	if _, ok := v["mcp_server_cmd"]; !ok {
		return v
	}
	out := make(map[string]interface{}, len(v)-1)
	for key, value := range v {
		if key != "mcp_server_cmd" {
			out[key] = value
		}
	}
	return out
}

func sanitizeMetadataWithOptions(v map[string]interface{}, options SQLiteOptions) map[string]interface{} {
	base := sanitizeMetadata(v)
	if options.CaptureMode == FullPayload {
		return base
	}
	keys := map[string]bool{"prompt": true, "messages": true, "arguments": true, "input": true, "output": true, "result": true, "content": true, "body": true, "authorization": true, "token": true, "api_key": true, "apikey": true, "secret": true}
	for _, key := range options.RedactKeys {
		keys[strings.ToLower(strings.TrimSpace(key))] = true
	}
	if options.CaptureMode == MetadataOnly {
		out := make(map[string]interface{}, len(base))
		for k, v := range base {
			lk := strings.ToLower(k)
			if keys[lk] {
				continue
			}
			out[k] = v
		}
		return out
	}
	return redactValue(base, keys).(map[string]interface{})
}

func redactValue(value interface{}, keys map[string]bool) interface{} {
	switch x := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, v := range x {
			if keys[strings.ToLower(k)] {
				out[k] = "[REDACTED]"
			} else {
				out[k] = redactValue(v, keys)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, v := range x {
			out[i] = redactValue(v, keys)
		}
		return out
	default:
		return value
	}
}

// Prune deletes expired raw events and rebuilds derived aggregates. It is
// safe to run while readers are active and is intentionally explicit so
// operators can schedule it as part of their retention policy.
func (s *SQLiteStorage) Prune(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.db == nil {
		return fmt.Errorf("storage not initialized")
	}
	if s.options.RetentionDays <= 0 && s.options.MaxBytes <= 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if s.options.RetentionDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -s.options.RetentionDays).Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, `DELETE FROM events WHERE timestamp < ?`, cutoff); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE session_id NOT IN (SELECT DISTINCT session_id FROM events)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tools WHERE tool_name NOT IN (SELECT DISTINCT tool_name FROM events)`); err != nil {
		return err
	}
	// Rebuild derived tables so retention never leaves stale success rates or
	// latency totals behind.
	for _, stmt := range []string{
		`DELETE FROM sessions`, `DELETE FROM agents`, `DELETE FROM tools`, `DELETE FROM daily_stats`,
		`INSERT INTO sessions (session_id, agent_name, adapter_name, first_seen, last_seen, call_count) SELECT session_id, MAX(agent_name), MAX(adapter_name), MIN(timestamp), MAX(timestamp), COUNT(*) FROM events GROUP BY session_id`,
		`INSERT INTO agents (agent_name, agent_version, call_count, success_count, total_duration_ms, input_size, output_size, last_seen) SELECT agent_name, MAX(agent_version), COUNT(*), COALESCE(SUM(success),0), COALESCE(SUM(duration_ms),0), COALESCE(SUM(input_size),0), COALESCE(SUM(output_size),0), MAX(timestamp) FROM events GROUP BY agent_name`,
		`INSERT INTO tools (tool_name, tool_type, call_count, success_count, total_duration_ms, input_size, output_size, last_seen) SELECT tool_name, MAX(tool_type), COUNT(*), COALESCE(SUM(success),0), COALESCE(SUM(duration_ms),0), COALESCE(SUM(input_size),0), COALESCE(SUM(output_size),0), MAX(timestamp) FROM events GROUP BY tool_name`,
		`INSERT INTO daily_stats (stat_day, call_count, success_count, total_duration_ms, input_size, output_size) SELECT substr(timestamp,1,10), COUNT(*), COALESCE(SUM(success),0), COALESCE(SUM(duration_ms),0), COALESCE(SUM(input_size),0), COALESCE(SUM(output_size),0) FROM events GROUP BY substr(timestamp,1,10)`,
	} {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if s.options.MaxBytes > 0 {
		if info, err := os.Stat(s.path); err == nil && info.Size() > s.options.MaxBytes {
			return fmt.Errorf("database size %d exceeds limit %d after retention", info.Size(), s.options.MaxBytes)
		}
	}
	return nil
}

func unmarshalMetadata(v string) (map[string]interface{}, error) {
	if v == "" {
		return map[string]interface{}{}, nil
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(v), &out); err != nil {
		return nil, err
	}
	if out == nil {
		return map[string]interface{}{}, nil
	}
	return out, nil
}
