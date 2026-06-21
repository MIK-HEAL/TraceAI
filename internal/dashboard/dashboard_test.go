package dashboard

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
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

	srv := httptest.NewServer(New(store).Handler(""))
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

func TestDashboardTokenAuth(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	srv := httptest.NewServer(New(store).Handler("secret"))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	req, err = http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer secret")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected ok, got %d", resp.StatusCode)
	}
}

func TestDashboardServeStopsOnContextCancel(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- New(store).ServeListener(ctx, ln, "")
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("expected graceful shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected server to stop after context cancel")
	}
}

func TestDashboardRequiresTokenForNonLocalBind(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	err := New(store).Serve(context.Background(), "0.0.0.0:0", "")
	if err == nil {
		t.Fatal("expected non-local bind without token to fail")
	}
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
