package traceai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
)

func TestRecordStartAndFinish(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	ctx := RecordStart(context.Background(), client, CallInfo{
		AdapterName:  "test",
		AgentName:    "agent",
		ToolName:     "tool",
		ToolType:     "tool",
		FunctionName: "call",
	})
	if err := RecordFinish(ctx, true, 1, 2, nil); err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
	if store.events[0].ToolName != "tool" {
		t.Fatalf("unexpected tool name: %+v", store.events[0])
	}
}

func TestHTTPMiddlewareRecordsStatusAndSizes(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	handler := HTTPMiddleware(client, CallInfo{
		AdapterName:  "http",
		AgentName:    "agent",
		ToolType:     "http",
		ToolName:     "health",
		FunctionName: "GET /health",
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
	got := store.events[0]
	if got.Success != true || got.OutputSize != 2 {
		t.Fatalf("unexpected http event: %+v", got)
	}
}

func TestUnaryInterceptorRecords(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	interceptor := UnaryServerInterceptor(client, CallInfo{
		AdapterName:  "grpc",
		AgentName:    "agent",
		ToolType:     "grpc",
		ToolName:     "repo",
		FunctionName: "ListFiles",
	})

	info := &grpc.UnaryServerInfo{FullMethod: "/traceai.Repo/ListFiles"}
	resp, err := interceptor(context.Background(), map[string]string{"a": "b"}, info, func(ctx context.Context, req any) (any, error) {
		return map[string]string{"ok": "yes"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("expected response")
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
}

func TestWrapMCPRecords(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	wrapped := WrapMCP(client, CallInfo{
		AdapterName:  "mcp",
		AgentName:    "agent",
		ToolType:     "mcp",
		ToolName:     "github",
		FunctionName: "search_code",
	}, func(context.Context) error {
		return errors.New("boom")
	})

	err := wrapped(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
	if store.events[0].ErrorMessage != "boom" {
		t.Fatalf("unexpected error message: %+v", store.events[0])
	}
}

func TestTraceaiClientPublishStillWorks(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	client := &Client{Store: store}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = client.Close(5 * time.Second)
	})

	if err := client.Publish(models.ToolEvent{
		AdapterName:  "traceai",
		AgentName:    "demo-agent",
		ToolName:     "demo",
		ToolType:     "mcp",
		FunctionName: "call",
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := store.ListEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one event, got %d", len(rows))
	}
	if rows[0].EventID == "" || rows[0].TraceID == "" || rows[0].SessionID == "" {
		t.Fatalf("expected generated ids, got %+v", rows[0])
	}
}

func TestStreamInterceptorRecords(t *testing.T) {
	store := &stubStore{}
	client := &Client{Store: store, Export: &stubExporter{}}
	interceptor := StreamServerInterceptor(client, CallInfo{
		AdapterName:  "grpc",
		AgentName:    "agent",
		ToolType:     "grpc",
		ToolName:     "repo",
		FunctionName: "ListFiles",
	})

	stream := &testServerStream{
		ctx:  context.Background(),
		recv: []any{map[string]string{"path": "README.md"}},
	}
	err := interceptor(nil, stream, &grpc.StreamServerInfo{FullMethod: "/traceai.Repo/ListFiles", IsClientStream: true, IsServerStream: true}, func(_ any, s grpc.ServerStream) error {
		var req map[string]string
		if err := s.RecvMsg(&req); err != nil {
			return err
		}
		return s.SendMsg(map[string]string{"ok": "yes"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected one event, got %d", len(store.events))
	}
	if store.events[0].InputSize == 0 || store.events[0].OutputSize == 0 {
		t.Fatalf("expected sizes to be recorded: %+v", store.events[0])
	}
}

type testServerStream struct {
	ctx  context.Context
	recv []any
	sent []any
}

func (s *testServerStream) SetHeader(metadata.MD) error  { return nil }
func (s *testServerStream) SendHeader(metadata.MD) error { return nil }
func (s *testServerStream) SetTrailer(metadata.MD)       {}
func (s *testServerStream) Context() context.Context     { return s.ctx }
func (s *testServerStream) SendMsg(m any) error {
	s.sent = append(s.sent, m)
	return nil
}
func (s *testServerStream) RecvMsg(m any) error {
	if len(s.recv) == 0 {
		return errors.New("eof")
	}
	msg := s.recv[0]
	s.recv = s.recv[1:]
	switch dst := m.(type) {
	case *map[string]string:
		*dst = msg.(map[string]string)
	}
	return nil
}
