package sdk

import (
	"context"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

func TestEndToEndSmoke(t *testing.T) {
	store := storage.NewMemoryStorage()
	client := New(store)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}

	client.Publish(buildEvent("mcp", "search", "tool_call", "demo-agent", true, 120, 80, 140))
	client.Publish(buildEvent("openai", "chat.completions", "function_call", "demo-agent", true, 210, 256, 512))

	if err := client.Close(2 * time.Second); err != nil {
		t.Fatal(err)
	}

	eventsRows, err := store.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsRows) != 2 {
		t.Fatalf("expected 2 events, got %d", len(eventsRows))
	}

	topTools, err := client.TopTools(ctx, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(topTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(topTools))
	}

	monthly, err := client.MonthlyStats(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(monthly) != 1 {
		t.Fatalf("expected 1 monthly row, got %d", len(monthly))
	}

	stats, err := client.Engine.Stats(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Calls != 2 {
		t.Fatalf("expected 2 calls, got %+v", stats)
	}
}

func buildEvent(adapterName, toolName, functionName, agentName string, success bool, durationMS, inputSize, outputSize int64) events.ToolEvent {
	event := events.NewToolEvent()
	event.AdapterName = adapterName
	event.AdapterVersion = "0.1.0"
	event.AgentName = agentName
	event.AgentVersion = "0.1.0"
	event.ToolType = adapterName
	event.ToolName = toolName
	event.FunctionName = functionName
	event.Success = success
	event.DurationMS = durationMS
	event.InputSize = inputSize
	event.OutputSize = outputSize
	return event
}
