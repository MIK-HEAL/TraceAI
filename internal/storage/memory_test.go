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
	event.ErrorCode = ""
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

func TestMemoryStorageDailyStats(t *testing.T) {
	store := NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.ToolType = "mcp"
	event.ToolName = "search"
	event.FunctionName = "tool_call"
	event.Timestamp = time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)
	event.Success = true
	event.DurationMS = 100
	event.InputSize = 12
	event.OutputSize = 34
	if err := store.InsertEvent(context.Background(), event); err != nil {
		t.Fatal(err)
	}

	rows, err := store.DailyStats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 daily stat row, got %d", len(rows))
	}
	if rows[0].StatDay != "2026-06-21" || rows[0].Calls != 1 {
		t.Fatalf("unexpected daily stats: %+v", rows[0])
	}
}

func TestMemoryStorageMonthlyStats(t *testing.T) {
	store := NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	for _, ts := range []time.Time{
		time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC),
	} {
		event := events.NewToolEvent()
		event.AdapterName = "mcp"
		event.ToolType = "mcp"
		event.ToolName = "search"
		event.FunctionName = "tool_call"
		event.Timestamp = ts
		event.Success = true
		event.DurationMS = 100
		event.InputSize = 12
		event.OutputSize = 34
		if err := store.InsertEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	rows, err := store.MonthlyStats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 monthly stat rows, got %d", len(rows))
	}
	if rows[0].StatMonth != "2026-05" || rows[1].StatMonth != "2026-06" {
		t.Fatalf("unexpected monthly stats: %+v", rows)
	}
}
