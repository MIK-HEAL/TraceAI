package storage

import (
	"context"
	"time"

	"toollens/internal/events"
)

type Storage interface {
	Init(ctx context.Context) error
	Close() error
	InsertEvent(ctx context.Context, event events.ToolEvent) error
	ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error)
	TopTools(ctx context.Context, since time.Time, limit int) ([]ToolCount, error)
	TopFunctions(ctx context.Context, since time.Time, limit int) ([]FunctionCount, error)
	TopAgents(ctx context.Context, since time.Time, limit int) ([]AgentCount, error)
	ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]ToolFailureRate, error)
	Stats(ctx context.Context, since time.Time) (Stats, error)
	DailyStats(ctx context.Context, since time.Time) ([]DailyStat, error)
}

type ToolCount struct {
	ToolName string
	Calls    int64
	Success  int64
}

type ToolFailureRate struct {
	ToolName    string
	Calls       int64
	Failures    int64
	FailureRate float64
}

type FunctionCount struct {
	FunctionName string
	Calls        int64
	Success      int64
}

type AgentCount struct {
	AgentName string
	Calls     int64
	Success   int64
}

type countedItem struct {
	Key     string
	Calls   int64
	Success int64
}

type Stats struct {
	Calls       int64
	SuccessRate float64
	AvgLatency  float64
	InputSize   int64
	OutputSize  int64
}

type DailyStat struct {
	StatDay         string
	Calls           int64
	Success         int64
	TotalDurationMS int64
	InputSize       int64
	OutputSize      int64
}
