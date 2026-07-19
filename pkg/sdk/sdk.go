package sdk

import (
	"context"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/analytics"
	"github.com/MIK-HEAL/TraceAI/internal/collector"
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
	"github.com/MIK-HEAL/TraceAI/pkg/state"
)

type SDK struct {
	Store     storage.Storage
	Collector *collector.Collector
	Engine    *analytics.Engine
}

func New(store storage.Storage) *SDK {
	c := collector.NewCollector(store)
	return &SDK{
		Store:     store,
		Collector: c,
		Engine:    analytics.NewEngine(store),
	}
}

func (s *SDK) Start(ctx context.Context) error {
	if err := s.Store.Init(ctx); err != nil {
		return err
	}
	return s.Collector.Start(ctx)
}

func (s *SDK) Publish(event events.ToolEvent) error {
	return s.Collector.Publish(event)
}

func (s *SDK) Close(timeout time.Duration) error {
	return s.Collector.Close(timeout)
}

func (s *SDK) TopTools(ctx context.Context, since time.Time, limit int) ([]storage.ToolCount, error) {
	return s.Engine.TopTools(ctx, since, limit)
}

func (s *SDK) MonthlyStats(ctx context.Context, since time.Time) ([]storage.MonthlyStat, error) {
	return s.Engine.MonthlyStats(ctx, since)
}

func (s *SDK) WeeklyStats(ctx context.Context, since time.Time) ([]storage.WeeklyStat, error) {
	return s.Engine.WeeklyStats(ctx, since)
}

func (s *SDK) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]storage.ErrorBreakdown, error) {
	return s.Engine.ErrorBreakdowns(ctx, since, limit)
}

func (s *SDK) Status(ctx context.Context) (state.Status, error) {
	status := state.Status{
		CheckedAt: time.Now().UTC(),
	}
	if err := s.Store.Ping(ctx); err != nil {
		status.LastError = err.Error()
		return status, err
	}
	status.StorageOK = true
	status.QueueClosed = s.Collector.Bus.IsClosed()
	status.QueueLen = len(s.Collector.Bus.Input())
	status.QueueCap = cap(s.Collector.Bus.Input())
	if err := s.Collector.Bus.LastError(); err != nil {
		status.LastError = err.Error()
	}
	return status, nil
}

func (s *SDK) Metrics(ctx context.Context) ([]state.Metric, error) {
	stats, err := s.Engine.Stats(ctx, time.Time{})
	if err != nil {
		return nil, err
	}
	queueLen := float64(len(s.Collector.Bus.Input()))
	queueCap := float64(cap(s.Collector.Bus.Input()))
	lastError := 0.0
	if s.Collector.Bus.LastError() != nil {
		lastError = 1
	}
	return []state.Metric{
		{Name: "calls", Value: float64(stats.Calls)},
		{Name: "success_rate", Value: stats.SuccessRate},
		{Name: "avg_latency_ms", Value: stats.AvgLatency},
		{Name: "input_size", Value: float64(stats.InputSize)},
		{Name: "output_size", Value: float64(stats.OutputSize)},
		{Name: "queue_len", Value: queueLen},
		{Name: "queue_cap", Value: queueCap},
		{Name: "last_error_present", Value: lastError},
	}, nil
}
