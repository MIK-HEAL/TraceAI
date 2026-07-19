package traceai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
	"github.com/MIK-HEAL/TraceAI/pkg/semantic"
)

type stubStore struct {
	events []models.ToolEvent
}

func (s *stubStore) Init(context.Context) error { return nil }
func (s *stubStore) Close() error               { return nil }
func (s *stubStore) Ping(context.Context) error { return nil }

func (s *stubStore) InsertEvent(_ context.Context, event models.ToolEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubStore) ListEvents(context.Context, int) ([]models.ToolEvent, error) { return nil, nil }
func (s *stubStore) TopTools(context.Context, time.Time, int) ([]models.ToolCount, error) {
	return nil, nil
}
func (s *stubStore) TopFunctions(context.Context, time.Time, int) ([]models.FunctionCount, error) {
	return nil, nil
}
func (s *stubStore) TopAgents(context.Context, time.Time, int) ([]models.AgentCount, error) {
	return nil, nil
}
func (s *stubStore) ToolFailureRates(context.Context, time.Time, int) ([]models.ToolFailureRate, error) {
	return nil, nil
}
func (s *stubStore) Stats(context.Context, time.Time) (models.Stats, error) {
	return models.Stats{}, nil
}
func (s *stubStore) DailyStats(context.Context, time.Time) ([]models.DailyStat, error) {
	return nil, nil
}
func (s *stubStore) MonthlyStats(context.Context, time.Time) ([]models.MonthlyStat, error) {
	return nil, nil
}
func (s *stubStore) WeeklyStats(context.Context, time.Time) ([]models.WeeklyStat, error) {
	return nil, nil
}
func (s *stubStore) ErrorBreakdowns(context.Context, time.Time, int) ([]models.ErrorBreakdown, error) {
	return nil, nil
}

type stubExporter struct {
	events []models.ToolEvent
}

func (s *stubExporter) Export(event models.ToolEvent) error {
	s.events = append(s.events, event)
	return nil
}

func (s *stubExporter) Close() error { return nil }

func TestClientPublishNormalizesAndExports(t *testing.T) {
	store := &stubStore{}
	exporter := &stubExporter{}
	client := &Client{Store: store, Export: exporter}

	event := models.ToolEvent{
		ToolName:     "search_code",
		ToolType:     "mcp",
		AgentName:    "demo-agent",
		FunctionName: "search_code",
	}
	if err := client.Publish(event); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 stored event, got %d", len(store.events))
	}
	if len(exporter.events) != 1 {
		t.Fatalf("expected 1 exported event, got %d", len(exporter.events))
	}
	got := store.events[0]
	if got.SchemaVersion != models.SchemaVersion {
		t.Fatalf("expected schema version %q, got %q", models.SchemaVersion, got.SchemaVersion)
	}
	if got.Metadata == nil {
		t.Fatal("expected metadata to be normalized")
	}
}

func TestLocalExporterWritesJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	t.Setenv("TEMP", tmpDir)
	t.Setenv("TMP", tmpDir)

	exporter := NewLocalExporter()
	event := models.ToolEvent{ToolName: "search_code", ToolType: "mcp"}
	if err := exporter.Export(event); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmpDir, "traceai-events.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "search_code") {
		t.Fatalf("expected jsonl export content, got %s", string(data))
	}
}

func TestOTLPExporterWritesMappedJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	t.Setenv("TEMP", tmpDir)
	t.Setenv("TMP", tmpDir)

	exporter := NewOTLPExporter()
	event := models.ToolEvent{ToolName: "search_code", ToolType: "mcp", AgentName: "claude-code"}
	if err := exporter.Export(event); err != nil {
		t.Fatal(err)
	}
	if err := exporter.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(tmpDir, "traceai-otlp.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var otlp OTLPEvent
	if err := json.Unmarshal(data, &otlp); err != nil {
		t.Fatal(err)
	}
	if got := otlp.Attributes["traceai.tool.name"]; got != "search_code" {
		t.Fatalf("unexpected OTLP tool name: %#v", otlp.Attributes)
	}
	if got := otlp.Attributes["traceai.agent.name"]; got != "claude-code" {
		t.Fatalf("unexpected OTLP agent name: %#v", otlp.Attributes)
	}
}

func TestInterceptorWrapHTTPMapsSemanticFields(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	interceptor := Interceptor{
		Client: client,
		Info: CallInfo{
			TraceID:      "trc_http",
			SessionID:    "ses_http",
			AdapterName:  "http",
			AgentName:    "demo-agent",
			ToolType:     "http",
			ToolName:     "profile",
			FunctionName: "GET /health",
		},
	}

	handler := interceptor.WrapHTTP(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}), "http", "profile", "GET /health")

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/health", strings.NewReader("payload"))
	req.ContentLength = int64(len("payload"))
	rr := &responseRecorder{header: http.Header{}}
	handler.ServeHTTP(rr, req)

	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	event := store.events[0]
	if event.ToolName != "profile" {
		t.Fatalf("unexpected tool name: %q", event.ToolName)
	}
	if event.FunctionName != "GET /health" {
		t.Fatalf("unexpected function name: %q", event.FunctionName)
	}
	if event.TraceID != "trc_http" || event.SessionID != "ses_http" {
		t.Fatalf("legacy interceptor lost correlation IDs: %+v", event)
	}
	if !event.Success {
		t.Fatal("expected success event")
	}
	if event.InputSize != int64(len("payload")) || event.OutputSize != 2 {
		t.Fatalf("unexpected sizes: %+v", event)
	}
}

func TestPublicAPIWithRealMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	client := &Client{Store: store}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close(5 * time.Second)
	})

	if err := CaptureRPC(ctx, client, CallInfo{
		AdapterName:  "grpc",
		AgentName:    "claude-code",
		ToolType:     "grpc",
		ToolName:     "repo",
		FunctionName: "ListFiles",
	}, func(context.Context) (int64, int64, error) {
		return 4096, 8192, nil
	}); err != nil {
		t.Fatal(err)
	}

	handler := HTTPMiddleware(client, CallInfo{
		AdapterName:  "http",
		AgentName:    "demo-agent",
		ToolType:     "http",
		ToolName:     "health",
		FunctionName: "GET /health",
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/health", strings.NewReader("payload"))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	rows, err := store.ListEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 events, got %d", len(rows))
	}
	if rows[0].ToolName != "health" || rows[1].ToolName != "repo" {
		t.Fatalf("unexpected event order or tool names: %+v", rows)
	}
	if rows[0].InputSize != int64(len("payload")) || rows[0].OutputSize != 2 {
		t.Fatalf("unexpected http sizes: %+v", rows[0])
	}
	if rows[1].InputSize != 4096 || rows[1].OutputSize != 8192 {
		t.Fatalf("unexpected grpc sizes: %+v", rows[1])
	}
}

func TestInterceptorCaptureRPCAndWrapMCP(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	interceptor := Interceptor{
		Client: client,
		Info: CallInfo{
			AdapterName:  "grpc",
			AgentName:    "demo-agent",
			ToolType:     "grpc",
			ToolName:     "repo",
			FunctionName: "ListFiles",
		},
	}

	err := interceptor.CaptureRPC(context.Background(), "grpc", "repo", "ListFiles", func(context.Context) (int64, int64, error) {
		return 12, 24, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	wrapped := interceptor.WrapMCP("demo-agent", "search", "tool_call")(func() error {
		return errors.New("boom")
	})
	if err := wrapped(); err == nil {
		t.Fatal("expected wrapped error")
	}

	if len(store.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(store.events))
	}
	if store.events[0].ToolType != "grpc" {
		t.Fatalf("unexpected rpc tool type: %+v", store.events[0])
	}
	if store.events[1].ToolType != "mcp" {
		t.Fatalf("unexpected mcp tool type: %+v", store.events[1])
	}
	if store.events[1].ErrorMessage != "boom" {
		t.Fatalf("unexpected mcp error message: %+v", store.events[1])
	}
}

func TestToOTLPMappings(t *testing.T) {
	event := models.ToolEvent{
		ToolName:   "search_code",
		ToolType:   "mcp",
		AgentName:  "claude-code",
		Success:    true,
		DurationMS: 99,
	}
	otlp := ToOTLP(event)
	if otlp.Attributes[semantic.ToolName] != "search_code" {
		t.Fatalf("unexpected tool name attribute: %+v", otlp.Attributes)
	}
	if otlp.Attributes[semantic.ToolSuccess] != true {
		t.Fatalf("unexpected success attribute: %+v", otlp.Attributes)
	}
	if otlp.Attributes[semantic.ToolDuration] != int64(99) {
		t.Fatalf("unexpected duration attribute: %+v", otlp.Attributes)
	}
	for _, key := range semantic.All {
		if _, ok := otlp.Attributes[key]; !ok {
			t.Fatalf("missing semantic key %q in otlp mapping: %+v", key, otlp.Attributes)
		}
	}
}

type responseRecorder struct {
	header     http.Header
	statusCode int
	body       []byte
}

func (r *responseRecorder) Header() http.Header { return r.header }
func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}
func (r *responseRecorder) Write(p []byte) (int, error) {
	r.body = append(r.body, p...)
	return len(p), nil
}
