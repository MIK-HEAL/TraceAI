package storage

import (
	"context"
	"testing"
	"time"

	"toollens/internal/events"
)

func TestMemoryStorageStats(t *testing.T) {
	store := NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.ToolType = "mcp"
	event.ToolName = "search"
	event.FunctionName = "tool_call"
	event.Success = true
	event.DurationMS = 100
	event.InputSize = 12
	event.OutputSize = 34
	if err := store.InsertEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	stats, err := store.Stats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Calls != 1 || stats.SuccessRate != 1 || stats.AvgLatency != 100 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}
