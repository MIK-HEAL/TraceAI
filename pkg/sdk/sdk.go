package sdk

import (
	"context"
	"time"

	"toollens/internal/analytics"
	"toollens/internal/collector"
	"toollens/internal/events"
	"toollens/internal/storage"
)

type SDK struct {
	Store    storage.Storage
	Collector *collector.Collector
	Engine   *analytics.Engine
}

func New(store storage.Storage) *SDK {
	c := collector.NewCollector(store)
	return &SDK{
		Store:    store,
		Collector: c,
		Engine:   analytics.NewEngine(store),
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
