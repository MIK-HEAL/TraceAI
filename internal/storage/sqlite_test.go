package storage

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
)

func TestSQLiteStorageMatchesMemoryAggregates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "trace.db")

	memory := NewMemoryStorage()
	sqlite := NewSQLiteStorage(dbPath)
	if err := memory.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer memory.Close()
	defer sqlite.Close()

	seed := []events.ToolEvent{
		buildStorageEvent("demo", "search", "read_file", true, time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)),
		buildStorageEvent("demo", "search", "read_file", false, time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)),
		buildStorageEvent("demo", "write", "write_file", false, time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)),
	}
	seed[1].ErrorType = "validation_error"
	seed[1].ErrorCode = "bad_input"
	seed[1].ErrorMessage = "invalid parameter"
	seed[2].ErrorType = "permission_error"
	seed[2].ErrorCode = "forbidden"
	seed[2].ErrorMessage = "access denied"

	for _, event := range seed {
		if err := memory.InsertEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
		if err := sqlite.InsertEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}

	memStats, err := memory.Stats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	sqlStats, err := sqlite.Stats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if memStats.Calls != sqlStats.Calls || memStats.SuccessRate != sqlStats.SuccessRate {
		t.Fatalf("stats mismatch: mem=%+v sqlite=%+v", memStats, sqlStats)
	}

	memMonthly, err := memory.MonthlyStats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	sqlMonthly, err := sqlite.MonthlyStats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(memMonthly) != len(sqlMonthly) {
		t.Fatalf("monthly stats mismatch: mem=%+v sqlite=%+v", memMonthly, sqlMonthly)
	}

	memWeekly, err := memory.WeeklyStats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	sqlWeekly, err := sqlite.WeeklyStats(context.Background(), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(memWeekly) != len(sqlWeekly) {
		t.Fatalf("weekly stats mismatch: mem=%+v sqlite=%+v", memWeekly, sqlWeekly)
	}

	memErrors, err := memory.ErrorBreakdowns(context.Background(), time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	sqlErrors, err := sqlite.ErrorBreakdowns(context.Background(), time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memErrors) != len(sqlErrors) {
		t.Fatalf("error breakdown mismatch: mem=%+v sqlite=%+v", memErrors, sqlErrors)
	}
}

func TestSQLiteCloseIsIdempotent(t *testing.T) {
	sqlite := NewSQLiteStorage(filepath.Join(t.TempDir(), "trace.db"))
	if err := sqlite.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.Close(); err != nil {
		t.Fatal(err)
	}
}

func buildStorageEvent(agent, tool, function string, success bool, ts time.Time) events.ToolEvent {
	event := events.NewToolEvent()
	event.AgentName = agent
	event.AgentVersion = "1.0.0"
	event.AdapterName = "mcp"
	event.AdapterVersion = "1.0.0"
	event.ToolType = "mcp"
	event.ToolName = tool
	event.FunctionName = function
	event.Success = success
	event.Timestamp = ts
	event.DurationMS = 100
	event.InputSize = 10
	event.OutputSize = 20
	return event
}
