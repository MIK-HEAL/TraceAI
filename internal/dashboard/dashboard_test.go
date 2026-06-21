package dashboard

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"toollens/internal/events"
	"toollens/internal/storage"
)

func TestDashboardPages(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.InsertEvent(context.Background(), testEvent("demo-agent", "mcp", "search", "search_code", true, "")); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertEvent(context.Background(), testEvent("demo-agent", "mcp", "write", "write_file", false, "permission denied")); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(New(store).Handler())
	defer srv.Close()

	assertPage := func(path, want string) {
		t.Helper()
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d: %s", path, resp.StatusCode, string(body))
		}
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %s to contain %q, got: %s", path, want, string(body))
		}
	}

	assertPage("/", "Overview")
	assertPage("/tools", "Tool Heatmap")
	assertPage("/agents", "Behavior Profile")
	assertPage("/errors", "Recent Failures")
}

func testEvent(agent, adapter, toolName, functionName string, success bool, message string) events.ToolEvent {
	event := events.NewToolEvent()
	event.Timestamp = time.Now().UTC()
	event.AgentName = agent
	event.AgentVersion = "1.0.0"
	event.AdapterName = adapter
	event.AdapterVersion = "1.0.0"
	event.ToolType = "mcp"
	event.ToolName = toolName
	event.FunctionName = functionName
	event.Success = success
	event.ErrorMessage = message
	event.DurationMS = 100
	return event
}
