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

func (c *Collector) Publish(event events.ToolEvent) error {
	return c.Bus.Publish(event)
}

// RecordToolCatalog persists discovery independently from execution events so
// zero-call tools remain visible to analytics without inflating call counts.
func (c *Collector) RecordToolCatalog(ctx context.Context, sessionID, serverName string, tools []storage.CatalogTool) error {
	if c == nil || c.Bus == nil {
		return nil
	}
	if store, ok := c.Bus.Storage().(storage.CatalogStorage); ok {
		return store.UpsertToolCatalog(ctx, sessionID, serverName, tools)
	}
	return nil
}

func (c *Collector) Close(timeout time.Duration) error {
	return c.Bus.CloseWithTimeout(timeout)
}
