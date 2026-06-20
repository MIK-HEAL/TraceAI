package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"toollens/internal/events"
)

type SQLiteStorage struct {
	path string
	db   *sql.DB
}

func NewSQLiteStorage(path string) *SQLiteStorage {
	if path == "" {
		path = "toollens.db"
	}
	return &SQLiteStorage{path: path}
}

func (s *SQLiteStorage) Init(ctx context.Context) error {
	db, err := sql.Open("sqlite", s.path)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	if err := s.migrate(ctx, db); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return nil
}

func (s *SQLiteStorage) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *SQLiteStorage) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	if err := event.Validate(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO events (
			event_id, schema_version, trace_id, session_id, timestamp,
			agent_name, agent_version, adapter_name, adapter_version,
			tool_type, tool_name, function_name, success, duration_ms,
			input_size, output_size, retry_count, error_type, error_code, error_message, metadata
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, event.EventID, event.SchemaVersion, event.TraceID, event.SessionID, event.Timestamp.UTC().Format(time.RFC3339Nano),
		event.AgentName, event.AgentVersion, event.AdapterName, event.AdapterVersion,
		event.ToolType, event.ToolName, event.FunctionName, boolToInt(event.Success), event.DurationMS,
		event.InputSize, event.OutputSize, event.RetryCount, event.ErrorType, event.ErrorCode, event.ErrorMessage, mustJSON(event.Metadata)); err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO sessions (session_id, agent_name, adapter_name, first_seen, last_seen, call_count)
		VALUES (?, ?, ?, ?, ?, 1)
		ON CONFLICT(session_id) DO UPDATE SET
			last_seen = excluded.last_seen,
			call_count = call_count + 1
	`, event.SessionID, event.AgentName, event.AdapterName, event.Timestamp.UTC().Format(time.RFC3339Nano), event.Timestamp.UTC().Format(time.RFC3339Nano)); err != nil {
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
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStorage) ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event_id, schema_version, trace_id, session_id, timestamp,
			agent_name, agent_version, adapter_name, adapter_version,
			tool_type, tool_name, function_name, success, duration_ms,
			input_size, output_size, retry_count, error_type, error_code, error_message, metadata
		FROM events
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
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
			&event.EventID, &event.SchemaVersion, &event.TraceID, &event.SessionID, &timestamp,
			&event.AgentName, &event.AgentVersion, &event.AdapterName, &event.AdapterVersion,
			&event.ToolType, &event.ToolName, &event.FunctionName, &success, &event.DurationMS,
			&event.InputSize, &event.OutputSize, &event.RetryCount, &event.ErrorType, &event.ErrorCode, &event.ErrorMessage, &metadata,
		); err != nil {
			return nil, err
		}
		if parsed, err := time.Parse(time.RFC3339Nano, timestamp); err == nil {
			event.Timestamp = parsed
		}
		event.Success = success == 1
		event.Metadata = parseJSON(metadata)
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

func (s *SQLiteStorage) Stats(ctx context.Context, since time.Time) (Stats, error) {
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

func (s *SQLiteStorage) topCounts(ctx context.Context, since time.Time, limit int, baseQuery, column string) ([]ToolCount, error) {
	where, args := sinceClause(since)
	query := fmt.Sprintf(`
		%s %s
		GROUP BY %s
		ORDER BY calls DESC, name ASC
		LIMIT ?
	`, baseQuery, where, column)
	args = append(args, limit)
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
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);`,
		`CREATE INDEX IF NOT EXISTS idx_events_tool_name ON events(tool_name);`,
		`CREATE INDEX IF NOT EXISTS idx_events_agent_name ON events(agent_name);`,
	}
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func sinceClause(since time.Time) (string, []any) {
	if since.IsZero() {
		return "", nil
	}
	return "WHERE timestamp >= ?", []any{since.UTC().Format(time.RFC3339Nano)}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func mustJSON(v map[string]interface{}) string {
	if len(v) == 0 {
		return "{}"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func parseJSON(v string) map[string]interface{} {
	if v == "" {
		return map[string]interface{}{}
	}
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(v), &out); err != nil || out == nil {
		return map[string]interface{}{}
	}
	return out
}
