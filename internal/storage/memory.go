package storage

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"toollens/internal/events"
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

func (s *MemoryStorage) InsertEvent(ctx context.Context, event events.ToolEvent) error {
	_ = ctx
	if err := event.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
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
