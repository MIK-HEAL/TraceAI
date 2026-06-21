package sdk

import (
	"context"
	"testing"
	"time"

	"toollens/internal/events"
	"toollens/internal/storage"
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

	waitForEvents(t, store, 2)
	client.Collector.Bus.Close()
	waitForEvents(t, store, 2)

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

	stats, err := client.Engine.Stats(ctx, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Calls != 2 {
		t.Fatalf("expected 2 calls, got %+v", stats)
	}
}

func waitForEvents(t *testing.T, store storage.Storage, expected int) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		eventsRows, err := store.ListEvents(context.Background(), 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(eventsRows) >= expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	eventsRows, err := store.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	t.Fatalf("expected at least %d events, got %d", expected, len(eventsRows))
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
