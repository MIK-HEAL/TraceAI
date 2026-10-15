package storage

import (
	"context"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
)

type Storage interface {
	Init(ctx context.Context) error
	Close() error
	Ping(ctx context.Context) error
	InsertEvent(ctx context.Context, event events.ToolEvent) error
	ListEvents(ctx context.Context, limit int) ([]events.ToolEvent, error)
	TopTools(ctx context.Context, since time.Time, limit int) ([]ToolCount, error)
	TopFunctions(ctx context.Context, since time.Time, limit int) ([]FunctionCount, error)
	TopAgents(ctx context.Context, since time.Time, limit int) ([]AgentCount, error)
	ToolFailureRates(ctx context.Context, since time.Time, limit int) ([]ToolFailureRate, error)
	Stats(ctx context.Context, since time.Time) (Stats, error)
	DailyStats(ctx context.Context, since time.Time) ([]DailyStat, error)
	MonthlyStats(ctx context.Context, since time.Time) ([]MonthlyStat, error)
	WeeklyStats(ctx context.Context, since time.Time) ([]WeeklyStat, error)
	ErrorBreakdowns(ctx context.Context, since time.Time, limit int) ([]ErrorBreakdown, error)

	// CallSequences returns the most frequent consecutive tool call chains.
	// depth controls the chain length: 2 = pairs, 3 = triples.
	CallSequences(ctx context.Context, since time.Time, depth, limit int) ([]CallSequence, error)

	// RetryPatterns analyses per-tool retry behaviour within sessions.
	RetryPatterns(ctx context.Context, since time.Time, limit int) ([]RetryPattern, error)
}

// CatalogStorage is implemented by stores that persist the MCP capability
// catalog separately from call events. Keeping it optional preserves the
// Storage interface for lightweight and third-party implementations.
type CatalogStorage interface {
	UpsertToolCatalog(ctx context.Context, sessionID, serverName string, tools []CatalogTool) error
	ToolCatalog(ctx context.Context, serverName string) ([]CatalogTool, error)
}

type CatalogTool struct {
	Name        string
	Description string
}

type UsageStorage interface {
	ToolUsage(ctx context.Context, since time.Time, limit int) ([]ToolUsage, error)
}
type ToolUsage struct {
	ToolName                                     string
	Calls, Success, Failures, Retries, ZeroCalls int64
	AvgDurationMS                                float64
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

type MonthlyStat struct {
	StatMonth       string
	Calls           int64
	Success         int64
	TotalDurationMS int64
	InputSize       int64
	OutputSize      int64
}

type WeeklyStat struct {
	StatWeek        string
	Calls           int64
	Success         int64
	TotalDurationMS int64
	InputSize       int64
	OutputSize      int64
}

type ErrorBreakdown struct {
	ErrorType string
	ErrorCode string
	Category  string
	Calls     int64
	Failures  int64
}

// CallSequence represents a consecutive tool call chain (e.g. "search -> read -> write").
type CallSequence struct {
	Sequence string // "tool_a -> tool_b -> tool_c"
	Count    int64
}

// RetryPattern describes how a tool behaves across retries within a session.
type RetryPattern struct {
	ToolName     string
	TotalCalls   int64
	Sessions     int64 // number of sessions where this tool appears
	NeverFails   int64 // all calls succeed
	AlwaysFails  int64 // all calls fail
	Recovers     int64 // fails then succeeds (recovery)
	Degrades     int64 // succeeds then fails (degradation)
	Intermittent int64 // mixed pattern with multiple transitions
}
