package test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/adapters"
	"github.com/MIK-HEAL/TraceAI/internal/collector"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

func TestMCPProxyInterceptsAndPersistsToolCalls(t *testing.T) {
	ctx := context.Background()
	store := storage.NewMemoryStorage()
	if err := store.Init(ctx); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col := collector.NewCollector(store)
	if err := col.Start(ctx); err != nil {
		t.Fatal(err)
	}

	input, writer := io.Pipe()
	transport := newFakeMCPTransport()
	output := &lockedBuffer{}
	proxy, err := adapters.NewMCPProxyWithTransport(
		&adapters.MCPProxyConfig{MCPCmd: "fake-mcp --token never-persist"},
		col,
		transport,
		input,
		output,
	)
	if err != nil {
		t.Fatal(err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- proxy.Run(ctx) }()

	for _, request := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search","arguments":{"query":"TraceAI"}}}`,
	} {
		if _, err := fmt.Fprintln(writer, request); err != nil {
			t.Fatal(err)
		}
	}

	waitFor(t, time.Second, func() bool { return output.lines() == 3 })
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("proxy did not stop after client input closed")
	}
	if err := col.Close(time.Second); err != nil {
		t.Fatal(err)
	}

	stored, err := store.ListEvents(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected one recorded tool event, got %d", len(stored))
	}
	event := stored[0]
	if event.ToolName != "search" || event.FunctionName != "tools/call" || !event.Success {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.AgentName != "mcp-client:fake-server" {
		t.Fatalf("expected captured agent name, got %q", event.AgentName)
	}
	if event.Metadata["mcp_server"] != "fake-server" || event.Metadata["mcp_server_command"] != "fake-mcp" {
		t.Fatalf("unexpected MCP metadata: %+v", event.Metadata)
	}
	if _, found := event.Metadata["mcp_server_cmd"]; found {
		t.Fatalf("raw command must never be persisted: %+v", event.Metadata)
	}
	if tools := proxy.ServerTools(); len(tools) != 1 || tools[0].Name != "search" {
		t.Fatalf("expected captured tool catalog, got %+v", tools)
	}
}

func TestMCPProxyCancellationUnblocksBothReaders(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	col := collector.NewCollector(store)
	if err := col.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer col.Close(time.Second)

	input, _ := io.Pipe()
	transport := newFakeMCPTransport()
	proxy, err := adapters.NewMCPProxyWithTransport(&adapters.MCPProxyConfig{}, col, transport, input, &lockedBuffer{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan error, 1)
	go func() { runDone <- proxy.Run(ctx) }()
	waitFor(t, time.Second, transport.isStarted)
	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("proxy did not stop after context cancellation")
	}
	if !transport.isClosed() {
		t.Fatal("expected transport to close on cancellation")
	}
}

func TestStdioTransportUsesSingleCancelableReader(t *testing.T) {
	if strings.ContainsAny(os.Args[0], " \t") {
		t.Skip("test helper executable path contains whitespace")
	}
	transport, err := adapters.NewStdioTransport(os.Args[0] + " -test.run=^TestMCPProxyHelperProcess$")
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer transport.Close()

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := transport.Receive(timeoutCtx); err == nil {
		t.Fatal("expected blocked receive to honor context cancellation")
	}

	message, err := adapters.ParseJSONRPCMessage([]byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Send(message); err != nil {
		t.Fatal(err)
	}
	response, err := transport.Receive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if response.Method != "ping" || response.IDString() != "1" {
		t.Fatalf("unexpected echoed message: %+v", response)
	}
}

// TestMCPProxyHelperProcess is invoked as a child process by the stdio
// transport test. In ordinary test runs it returns immediately.
func TestMCPProxyHelperProcess(t *testing.T) {
	helper := false
	for _, arg := range os.Args[1:] {
		if arg == "-test.run=^TestMCPProxyHelperProcess$" {
			helper = true
			break
		}
	}
	if !helper {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		_, _ = fmt.Fprintln(os.Stdout, scanner.Text())
	}
}

type fakeMCPTransport struct {
	mu        sync.Mutex
	started   bool
	closed    bool
	responses chan *adapters.JSONRPCMessage
}

func newFakeMCPTransport() *fakeMCPTransport {
	return &fakeMCPTransport{responses: make(chan *adapters.JSONRPCMessage, 8)}
}

func (t *fakeMCPTransport) Start(context.Context) error {
	t.mu.Lock()
	t.started = true
	t.mu.Unlock()
	return nil
}

func (t *fakeMCPTransport) Send(msg *adapters.JSONRPCMessage) error {
	switch msg.Method {
	case adapters.MCPMethodInitialize:
		t.responses <- response(msg.ID, `{"protocolVersion":"2025-03-26","serverInfo":{"name":"fake-server","version":"1.0.0"}}`)
	case adapters.MCPMethodToolsList:
		t.responses <- response(msg.ID, `{"tools":[{"name":"search","description":"Search source"}]}`)
	case adapters.MCPMethodToolsCall:
		t.responses <- response(msg.ID, `{"content":[{"type":"text","text":"ok"}]}`)
	}
	return nil
}

func (t *fakeMCPTransport) SendRaw([]byte) error { return nil }

func (t *fakeMCPTransport) Receive(ctx context.Context) (*adapters.JSONRPCMessage, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case msg, ok := <-t.responses:
		if !ok {
			return nil, io.EOF
		}
		return msg, nil
	}
}

func (t *fakeMCPTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.closed {
		t.closed = true
		close(t.responses)
	}
	return nil
}

func (t *fakeMCPTransport) isStarted() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.started
}

func (t *fakeMCPTransport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func response(id *json.RawMessage, result string) *adapters.JSONRPCMessage {
	return &adapters.JSONRPCMessage{JSONRPC: "2.0", ID: id, Result: json.RawMessage(result)}
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(data)
}

func (b *lockedBuffer) lines() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Count(b.b.Bytes(), []byte{'\n'})
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition was not satisfied before timeout")
}
