package test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/collector"
	"github.com/MIK-HEAL/TraceAI/internal/config"
	"github.com/MIK-HEAL/TraceAI/internal/dashboard"
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

func TestSQLiteAnalysisIncludesAllEvents(t *testing.T) {
	ctx := context.Background()
	store := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "traceai.db"))
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	start := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 150; i++ {
		tool := "read"
		if i%2 == 1 {
			tool = "write"
		}
		event := validEvent(tool)
		event.SessionID = "one-session"
		event.Timestamp = start.Add(time.Duration(i) * time.Millisecond)
		if err := store.InsertEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	all, err := store.ListEvents(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 150 {
		t.Fatalf("ListEvents(0) returned %d events, want all 150", len(all))
	}

	sequences, err := store.CallSequences(ctx, time.Time{}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(sequences) != 1 || sequences[0].Sequence != "read -> write" || sequences[0].Count != 75 {
		t.Fatalf("unexpected sequence analysis: %+v", sequences)
	}
}

func TestSQLiteRejectsNonSerializableMetadata(t *testing.T) {
	ctx := context.Background()
	store := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "traceai.db"))
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	event := validEvent("search")
	event.Metadata["callback"] = func() {}
	if err := store.InsertEvent(ctx, event); err == nil {
		t.Fatal("expected unsupported metadata to be rejected")
	}
}

func TestSQLiteDoesNotPersistLegacyMCPCommandMetadata(t *testing.T) {
	ctx := context.Background()
	store := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "traceai.db"))
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	event := validEvent("search")
	event.Metadata["mcp_server_cmd"] = "node server.js --token secret-value"
	if err := store.InsertEvent(ctx, event); err != nil {
		t.Fatal(err)
	}
	stored, err := store.ListEvents(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected one stored event, got %d", len(stored))
	}
	if _, found := stored[0].Metadata["mcp_server_cmd"]; found {
		t.Fatal("legacy MCP command metadata must not be persisted")
	}
}

func TestCollectorPublishSurfacesQueueFull(t *testing.T) {
	bus := collector.NewBus(storage.NewMemoryStorage(), 1, time.Hour)
	event := validEvent("search")
	for i := 0; i < 4; i++ {
		if err := bus.Publish(event); err != nil {
			t.Fatalf("unexpected enqueue error on item %d: %v", i, err)
		}
	}
	if err := bus.Publish(event); !errors.Is(err, collector.ErrQueueFull) {
		t.Fatalf("expected queue-full error, got %v", err)
	}
	bus.Close()
}

func TestValidateEventRejectsMissingIdentityAndNegativeMetrics(t *testing.T) {
	event := validEvent("search")
	event.TraceID = ""
	if err := event.Validate(); err == nil {
		t.Fatal("expected missing trace ID to be rejected")
	}
	event = validEvent("search")
	event.DurationMS = -1
	if err := event.Validate(); err == nil {
		t.Fatal("expected negative duration to be rejected")
	}
}

func TestConfigValidationRejectsSilentFallbacks(t *testing.T) {
	cfg := &config.Config{Store: "sqlite", DB: "trace.db", LogLevel: "verbose", LogFormat: "text"}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected unsupported log level to be rejected")
	}
	cfg = &config.Config{Store: "memory", LogLevel: "warning", LogFormat: "json"}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected warning alias to normalize to warn, got %q", cfg.LogLevel)
	}
}

func TestDashboardListenerRequiresTokenWhenPublic(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	listener, err := net.Listen("tcp4", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := dashboard.New(store).ServeListener(context.Background(), listener, ""); err == nil {
		t.Fatal("expected public listener without token to be rejected")
	}
}

func TestDashboardAuthenticationCreatesNavigationSession(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	handler := dashboard.New(store).Handler("secret")
	first := httptest.NewRequest(http.MethodGet, "/", nil)
	first.Header.Set("Authorization", "Bearer secret")
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, first)
	if firstResponse.Code != http.StatusOK {
		t.Fatalf("expected authenticated request to succeed, got %d", firstResponse.Code)
	}
	cookies := firstResponse.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("expected authenticated response to set a session cookie")
	}

	navigation := httptest.NewRequest(http.MethodGet, "/tools", nil)
	navigation.AddCookie(cookies[0])
	navigationResponse := httptest.NewRecorder()
	handler.ServeHTTP(navigationResponse, navigation)
	if navigationResponse.Code != http.StatusOK {
		t.Fatalf("expected navigation with session cookie to succeed, got %d", navigationResponse.Code)
	}
}

func validEvent(tool string) events.ToolEvent {
	event := events.NewToolEvent()
	event.AdapterName = "mcp"
	event.AdapterVersion = "1"
	event.ToolType = "mcp"
	event.ToolName = tool
	event.FunctionName = "tools/call"
	event.Timestamp = time.Now().UTC()
	return event
}
