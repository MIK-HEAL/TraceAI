package analytics

import (
	"context"
	"time"

	"toollens/internal/storage"
)

type Engine struct {
	Store storage.Storage
}

func NewEngine(store storage.Storage) *Engine {
	return &Engine{Store: store}
}

func (e *Engine) TopTools(ctx context.Context, since time.Time, limit int) ([]storage.ToolCount, error) {
	return e.Store.TopTools(ctx, since, limit)
}

func (e *Engine) TopFunctions(ctx context.Context, since time.Time, limit int) ([]storage.FunctionCount, error) {
	return e.Store.TopFunctions(ctx, since, limit)
}

func (e *Engine) TopAgents(ctx context.Context, since time.Time, limit int) ([]storage.AgentCount, error) {
	return e.Store.TopAgents(ctx, since, limit)
}

func (e *Engine) Stats(ctx context.Context, since time.Time) (storage.Stats, error) {
	return e.Store.Stats(ctx, since)
}
