package collector

import (
	"context"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

type Collector struct {
	Bus *Bus
}

func NewCollector(store storage.Storage) *Collector {
	return &Collector{
		Bus: NewBus(store, 32, 500*time.Millisecond),
	}
}

func (c *Collector) Start(ctx context.Context) error {
	return c.Bus.Start(ctx)
}

func (c *Collector) Publish(event events.ToolEvent) {
	c.Bus.Publish(event)
}

func (c *Collector) Close(timeout time.Duration) error {
	return c.Bus.CloseWithTimeout(timeout)
}
