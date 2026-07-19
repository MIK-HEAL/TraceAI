package storage

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
)

type MemoryStorage struct {
	mu     sync.RWMutex
	events []events.ToolEvent
	closed bool
}

func NewMemoryStorage() *MemoryStorage {
	return &MemoryStorage{}
}

func (s *MemoryStorage) Init(ctx context.Context) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = false
	s.events = nil
	return nil
}

func (s *MemoryStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *MemoryStorage) Ping(ctx context.Context) error {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		slog.Default().With("component", "storage", "backend", "memory").Warn("storage ping failed", "reason", "closed")
		return errors.New("storage closed")
	}
	return nil
}

func (s *MemoryStorage) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	_ = ctx
	event = event.Normalize()
	if err := event.Validate(); err != nil {
		slog.Default().With("component", "storage", "backend", "memory").Error("insert event failed", "event_id", event.EventID, "error", err)
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		slog.Default().With("component", "storage", "backend", "memory").Error("insert event failed", "reason", "closed", "event_id", event.EventID)
		return errors.New("storage closed")
	}
	s.events = append(s.events, event.Clone())
	return nil
}

func (s *MemoryStorage) ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	out := make([]events.ToolEvent, 0, limit)
	for i := len(s.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, s.events[i].Clone())
	}
	return out, nil
}

func (s *MemoryStorage) TopTools(ctx context.Context, since time.Time, limit int) ([]ToolCount, error) {
	return s.topCounts(ctx, since, limit, func(e events.ToolEvent) string { return e.ToolName })
}

func (s *MemoryStorage) TopFunctions(ctx context.Context, since time.Time, limit int) ([]FunctionCount, error) {
	items, err := s.topCounts(ctx, since, limit, func(e events.ToolEvent) string { return e.FunctionName })
	if err != nil {
		return nil, err
	}
	out := make([]FunctionCount, len(items))
	for i, item := range items {
		out[i] = FunctionCount{FunctionName: item.ToolName, Calls: item.Calls, Success: item.Success}
	}
	return out, nil
}

func (s *MemoryStorage) TopAgents(ctx context.Context, since time.Time, limit int) ([]AgentCount, error) {
	items, err := s.topCounts(ctx, since, limit, func(e events.ToolEvent) string { return e.AgentName })
	if err != nil {
		return nil, err
	}
	out := make([]AgentCount, len(items))
	for i, item := range items {
		out[i] = AgentCount{AgentName: item.ToolName, Calls: item.Calls, Success: item.Success}
	}
	return out, nil
}

func (s *MemoryStorage) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]ToolFailureRate, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	type counter struct {
		calls    int64
		failures int64
	}

	counts := map[string]*counter{}
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		if event.ToolName == "" {
			continue
		}
		item, ok := counts[event.ToolName]
		if !ok {
			item = &counter{}
			counts[event.ToolName] = item
		}
		item.calls++
		if !event.Success {
			item.failures++
		}
	}

	items := make([]ToolFailureRate, 0, len(counts))
	for toolName, item := range counts {
		failureRate := 0.0
		if item.calls > 0 {
			failureRate = float64(item.failures) / float64(item.calls)
		}
		items = append(items, ToolFailureRate{
			ToolName:    toolName,
			Calls:       item.calls,
			Failures:    item.failures,
			FailureRate: failureRate,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].FailureRate == items[j].FailureRate {
			if items[i].Calls == items[j].Calls {
				return items[i].ToolName < items[j].ToolName
			}
			return items[i].Calls > items[j].Calls
		}
		return items[i].FailureRate > items[j].FailureRate
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *MemoryStorage) Stats(ctx context.Context, since time.Time) (Stats, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	var stats Stats
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		stats.Calls++
		if event.Success {
			stats.SuccessRate++
		}
		stats.InputSize += event.InputSize
		stats.OutputSize += event.OutputSize
		stats.AvgLatency += float64(event.DurationMS)
	}
	if stats.Calls > 0 {
		stats.SuccessRate = stats.SuccessRate / float64(stats.Calls)
		stats.AvgLatency = stats.AvgLatency / float64(stats.Calls)
	}
	return stats, nil
}

func (s *MemoryStorage) DailyStats(ctx context.Context, since time.Time) ([]DailyStat, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := map[string]*DailyStat{}
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		day := event.Timestamp.UTC().Format("2006-01-02")
		item, ok := counts[day]
		if !ok {
			item = &DailyStat{StatDay: day}
			counts[day] = item
		}
		item.Calls++
		if event.Success {
			item.Success++
		}
		item.TotalDurationMS += event.DurationMS
		item.InputSize += event.InputSize
		item.OutputSize += event.OutputSize
	}

	items := make([]DailyStat, 0, len(counts))
	for _, item := range counts {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StatDay < items[j].StatDay
	})
	return items, nil
}

func (s *MemoryStorage) MonthlyStats(ctx context.Context, since time.Time) ([]MonthlyStat, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := map[string]*MonthlyStat{}
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		month := event.Timestamp.UTC().Format("2006-01")
		item, ok := counts[month]
		if !ok {
			item = &MonthlyStat{StatMonth: month}
			counts[month] = item
		}
		item.Calls++
		if event.Success {
			item.Success++
		}
		item.TotalDurationMS += event.DurationMS
		item.InputSize += event.InputSize
		item.OutputSize += event.OutputSize
	}

	items := make([]MonthlyStat, 0, len(counts))
	for _, item := range counts {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StatMonth < items[j].StatMonth
	})
	return items, nil
}

func (s *MemoryStorage) WeeklyStats(ctx context.Context, since time.Time) ([]WeeklyStat, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := map[string]*WeeklyStat{}
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		week := weekBucket(event.Timestamp)
		item, ok := counts[week]
		if !ok {
			item = &WeeklyStat{StatWeek: week}
			counts[week] = item
		}
		item.Calls++
		if event.Success {
			item.Success++
		}
		item.TotalDurationMS += event.DurationMS
		item.InputSize += event.InputSize
		item.OutputSize += event.OutputSize
	}

	items := make([]WeeklyStat, 0, len(counts))
	for _, item := range counts {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].StatWeek < items[j].StatWeek
	})
	return items, nil
}

func (s *MemoryStorage) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]ErrorBreakdown, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	type aggregate struct {
		ErrorBreakdown
	}

	counts := map[string]*aggregate{}
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		if event.Success && event.ErrorType == "" && event.ErrorCode == "" && event.ErrorMessage == "" {
			continue
		}
		category := classifyFailure(event.ErrorType, event.ErrorCode, event.ErrorMessage)
		key := fmt.Sprintf("%s|%s|%s", category, event.ErrorType, event.ErrorCode)
		item, ok := counts[key]
		if !ok {
			item = &aggregate{ErrorBreakdown: ErrorBreakdown{Category: category, ErrorType: event.ErrorType, ErrorCode: event.ErrorCode}}
			counts[key] = item
		}
		item.Calls++
		if !event.Success {
			item.Failures++
		}
	}

	items := make([]ErrorBreakdown, 0, len(counts))
	for _, item := range counts {
		items = append(items, item.ErrorBreakdown)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Failures == items[j].Failures {
			if items[i].Calls == items[j].Calls {
				if items[i].Category == items[j].Category {
					if items[i].ErrorType == items[j].ErrorType {
						return items[i].ErrorCode < items[j].ErrorCode
					}
					return items[i].ErrorType < items[j].ErrorType
				}
				return items[i].Category < items[j].Category
			}
			return items[i].Calls > items[j].Calls
		}
		return items[i].Failures > items[j].Failures
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *MemoryStorage) topCounts(ctx context.Context, since time.Time, limit int, keyFn func(events.ToolEvent) string) ([]ToolCount, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := map[string]*ToolCount{}
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		key := keyFn(event)
		if key == "" {
			continue
		}
		item, ok := counts[key]
		if !ok {
			item = &ToolCount{ToolName: key}
			counts[key] = item
		}
		item.Calls++
		if event.Success {
			item.Success++
		}
	}

	items := make([]ToolCount, 0, len(counts))
	for _, item := range counts {
		items = append(items, *item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Calls == items[j].Calls {
			return items[i].ToolName < items[j].ToolName
		}
		return items[i].Calls > items[j].Calls
	})
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func weekBucket(ts time.Time) string {
	year, week := ts.UTC().ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func classifyFailure(errorType, errorCode, errorMessage string) string {
	joined := strings.ToLower(strings.TrimSpace(strings.Join([]string{errorType, errorCode, errorMessage}, " ")))
	switch {
	case containsAny(joined, []string{"validation", "invalid", "parameter", "bad request", "schema", "parse", "format"}):
		return "parameter"
	case containsAny(joined, []string{"permission", "forbidden", "unauthorized", "auth", "access denied"}):
		return "permission"
	case containsAny(joined, []string{"timeout", "deadline", "context canceled", "context deadline", "unavailable", "connection", "network"}):
		return "context"
	default:
		return "other"
	}
}

func containsAny(value string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Call sequence and retry pattern analysis (M203)
// ---------------------------------------------------------------------------

func (s *MemoryStorage) CallSequences(ctx context.Context, since time.Time, depth, limit int) ([]CallSequence, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	if depth < 2 {
		depth = 2
	}
	if depth > 5 {
		depth = 5 // practical cap
	}

	// Group events by session, sorted by timestamp.
	sessions := s.groupBySession(since)

	// Count sequences across all sessions.
	seqCounts := make(map[string]int64)
	for _, events := range sessions {
		if len(events) < depth {
			continue
		}
		for i := 0; i <= len(events)-depth; i++ {
			parts := make([]string, depth)
			for j := 0; j < depth; j++ {
				parts[j] = events[i+j].ToolName
			}
			seq := strings.Join(parts, " -> ")
			seqCounts[seq]++
		}
	}

	// Sort by count descending.
	items := make([]CallSequence, 0, len(seqCounts))
	for seq, count := range seqCounts {
		items = append(items, CallSequence{Sequence: seq, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Sequence < items[j].Sequence
		}
		return items[i].Count > items[j].Count
	})

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *MemoryStorage) RetryPatterns(ctx context.Context, since time.Time, limit int) ([]RetryPattern, error) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Group events by session+tool.
	type sessionTool struct {
		sessionID string
		toolName  string
	}
	groups := make(map[sessionTool][]events.ToolEvent)
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		if event.ToolName == "" {
			continue
		}
		key := sessionTool{event.SessionID, event.ToolName}
		groups[key] = append(groups[key], event)
	}

	// Aggregate patterns per tool.
	type toolAgg struct {
		totalCalls   int64
		sessions     int64
		neverFails   int64
		alwaysFails  int64
		recovers     int64
		degrades     int64
		intermittent int64
	}
	agg := make(map[string]*toolAgg)

	for _, evts := range groups {
		if len(evts) == 0 {
			continue
		}
		// Sort by timestamp.
		sort.Slice(evts, func(i, j int) bool {
			return evts[i].Timestamp.Before(evts[j].Timestamp)
		})

		toolName := evts[0].ToolName
		a, ok := agg[toolName]
		if !ok {
			a = &toolAgg{}
			agg[toolName] = a
		}

		a.totalCalls += int64(len(evts))
		a.sessions++

		// Classify the pattern for this session+tool.
		pattern := classifyRetryPattern(evts)
		switch pattern {
		case "never_fails":
			a.neverFails++
		case "always_fails":
			a.alwaysFails++
		case "recovers":
			a.recovers++
		case "degrades":
			a.degrades++
		case "intermittent":
			a.intermittent++
		}
	}

	// Convert to sorted slice.
	items := make([]RetryPattern, 0, len(agg))
	for toolName, a := range agg {
		items = append(items, RetryPattern{
			ToolName:     toolName,
			TotalCalls:   a.totalCalls,
			Sessions:     a.sessions,
			NeverFails:   a.neverFails,
			AlwaysFails:  a.alwaysFails,
			Recovers:     a.recovers,
			Degrades:     a.degrades,
			Intermittent: a.intermittent,
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].TotalCalls == items[j].TotalCalls {
			return items[i].ToolName < items[j].ToolName
		}
		return items[i].TotalCalls > items[j].TotalCalls
	})

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// groupBySession groups events by session ID and sorts each group by timestamp.
func (s *MemoryStorage) groupBySession(since time.Time) map[string][]events.ToolEvent {
	groups := make(map[string][]events.ToolEvent)
	for _, event := range s.events {
		if !since.IsZero() && event.Timestamp.Before(since) {
			continue
		}
		if event.ToolName == "" {
			continue
		}
		groups[event.SessionID] = append(groups[event.SessionID], event)
	}
	// Sort each group by timestamp.
	for _, evts := range groups {
		sort.Slice(evts, func(i, j int) bool {
			return evts[i].Timestamp.Before(evts[j].Timestamp)
		})
	}
	return groups
}

// classifyRetryPattern analyses a sorted list of events for the same session+tool
// and classifies the retry behaviour.
func classifyRetryPattern(evts []events.ToolEvent) string {
	if len(evts) == 0 {
		return "never_fails"
	}

	allSuccess := true
	allFail := true
	transitions := 0
	prevSuccess := evts[0].Success

	for _, e := range evts {
		if e.Success {
			allFail = false
		} else {
			allSuccess = false
		}
		if e.Success != prevSuccess {
			transitions++
			prevSuccess = e.Success
		}
	}

	switch {
	case allSuccess:
		return "never_fails"
	case allFail:
		return "always_fails"
	case transitions == 1 && !evts[0].Success:
		return "recovers"
	case transitions == 1 && evts[0].Success:
		return "degrades"
	default:
		return "intermittent"
	}
}
