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

func (e *Engine) ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]storage.ToolFailureRate, error) {
	return e.Store.ToolFailureRates(ctx, since, limit)
}

func (e *Engine) Stats(ctx context.Context, since time.Time) (storage.Stats, error) {
	return e.Store.Stats(ctx, since)
}

func (e *Engine) DailyStats(ctx context.Context, since time.Time) ([]storage.DailyStat, error) {
	return e.Store.DailyStats(ctx, since)
}

func (e *Engine) MonthlyStats(ctx context.Context, since time.Time) ([]storage.MonthlyStat, error) {
	return e.Store.MonthlyStats(ctx, since)
}

func (e *Engine) WeeklyStats(ctx context.Context, since time.Time) ([]storage.WeeklyStat, error) {
	return e.Store.WeeklyStats(ctx, since)
}

func (e *Engine) ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]storage.ErrorBreakdown, error) {
	return e.Store.ErrorBreakdowns(ctx, since, limit)
}
