package sdk

import (
	"context"
	"time"

	"toollens/internal/analytics"
	"toollens/internal/collector"
	"toollens/internal/events"
	"toollens/internal/storage"
	"toollens/pkg/state"
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

func (s *SDK) Publish(event events.ToolEvent) {
	s.Collector.Publish(event)
}

func (s *SDK) TopTools(ctx context.Context, since time.Time, limit int) ([]storage.ToolCount, error) {
	return s.Engine.TopTools(ctx, since, limit)
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
