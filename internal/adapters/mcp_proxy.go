package adapters

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/collector"
	"github.com/MIK-HEAL/TraceAI/internal/events"
)

// ---------------------------------------------------------------------------
// Proxy configuration
// ---------------------------------------------------------------------------

// MCPProxyConfig holds all configuration for the MCP transparent proxy.
type MCPProxyConfig struct {
	// MCPCmd is the command that launches the real MCP server, e.g.
	// "npx -y @modelcontextprotocol/server-github".
	MCPCmd string

	// AgentName overrides the agent name recorded in events.
	// When empty, the proxy derives it from (in order):
	//   1. TRACEAI_AGENT_NAME env var
	//   2. The MCP server's name from the initialize handshake
	//   3. Fallback: "mcp-proxy"
	AgentName string

	// AdapterVersion is reported as the adapter version in events.
	AdapterVersion string
}

// withDefaults fills in any blank fields with sensible defaults.
func (c *MCPProxyConfig) withDefaults() *MCPProxyConfig {
	if c == nil {
		c = &MCPProxyConfig{}
	}
	if c.AdapterVersion == "" {
		c.AdapterVersion = "0.2.0"
	}
	if c.AgentName == "" {
		if name := os.Getenv("TRACEAI_AGENT_NAME"); name != "" {
			c.AgentName = name
		}
	}
	return c
}

// ---------------------------------------------------------------------------
// Pending call tracking
// ---------------------------------------------------------------------------

// pendingCall holds the start-time metadata for an in-flight tools/call.
type pendingCall struct {
	toolName    string
	startTime   time.Time
	requestSize int64
}

// ---------------------------------------------------------------------------
// Proxy
// ---------------------------------------------------------------------------

// MCPProxy is a transparent MCP proxy that sits between an AI agent and a
// real MCP server.  It intercepts tools/call requests/responses to record
// ToolEvents without requiring any changes to the agent or the server.
//
// All MCP protocol logic is confined to this file and its helpers
// (mcp_jsonrpc.go, mcp_transport_stdio.go).  The proxy publishes events
// through the standard collector.EventBus so that downstream components
// (storage, dashboard, CLI) operate on uniform ToolEvents without ever
// seeing MCP protocol details.
type MCPProxy struct {
	transport      MCPTransport
	collector      *collector.Collector
	adapterVersion string
	serverCommand  string

	sessionID   string
	serverName  string // discovered during initialize handshake
	agentName   string
	serverTools []MCPTool // tools discovered from tools/list

	pending        map[string]*pendingCall // tools/call requests, keyed by JSON-RPC id
	pendingMethods map[string]string       // control requests, keyed by JSON-RPC id
	mu             sync.Mutex

	input          io.ReadCloser
	output         io.Writer
	inputCloseOnce sync.Once
	closeOnce      sync.Once
	closeErr       error
}

// NewMCPProxy creates a proxy that will launch cmd as the real MCP server.
func NewMCPProxy(cfg *MCPProxyConfig, col *collector.Collector) (*MCPProxy, error) {
	cfg = cfg.withDefaults()
	if cfg.MCPCmd == "" {
		return nil, fmt.Errorf("MCPCmd is required")
	}

	transport, err := NewStdioTransport(cfg.MCPCmd)
	if err != nil {
		return nil, fmt.Errorf("create stdio transport: %w", err)
	}
	return newMCPProxy(cfg, col, transport, os.Stdin, os.Stdout)
}

// NewMCPProxyWithTransport creates a proxy around a caller-supplied transport
// and streams. It supports non-stdio transports and makes protocol handling
// independently testable without launching a child process.
func NewMCPProxyWithTransport(cfg *MCPProxyConfig, col *collector.Collector, transport MCPTransport, input io.ReadCloser, output io.Writer) (*MCPProxy, error) {
	cfg = cfg.withDefaults()
	return newMCPProxy(cfg, col, transport, input, output)
}

func newMCPProxy(cfg *MCPProxyConfig, col *collector.Collector, transport MCPTransport, input io.ReadCloser, output io.Writer) (*MCPProxy, error) {
	if col == nil {
		return nil, fmt.Errorf("collector is required")
	}
	if transport == nil {
		return nil, fmt.Errorf("mcp transport is required")
	}
	if input == nil {
		return nil, fmt.Errorf("proxy input is required")
	}
	if output == nil {
		return nil, fmt.Errorf("proxy output is required")
	}

	sessionID := events.NewToolEvent().SessionID

	proxy := &MCPProxy{
		transport:      transport,
		collector:      col,
		adapterVersion: cfg.AdapterVersion,
		serverCommand:  commandLabel(cfg.MCPCmd),
		sessionID:      sessionID,
		agentName:      cfg.AgentName,
		pending:        make(map[string]*pendingCall),
		pendingMethods: make(map[string]string),
		input:          input,
		output:         output,
	}

	slog.Default().With("component", "mcp_proxy", "session_id", sessionID, "server_command", proxy.serverCommand).Info("proxy created")
	return proxy, nil
}

// Run starts the proxy main loop.  It blocks until ctx is cancelled or
// an unrecoverable error occurs.
//
// The proxy runs two goroutines:
//   - serverReader: reads messages from the real MCP server, records
//     tool call completions, and forwards to os.Stdout (the AI agent).
//   - clientReader: reads messages from os.Stdin (the AI agent),
//     records tool call starts, and forwards to the real MCP server.
func (p *MCPProxy) Run(ctx context.Context) error {
	if err := p.transport.Start(ctx); err != nil {
		return fmt.Errorf("start transport: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	readerDone := make(chan struct{}, 2)

	// Goroutine 1: server → client (real MCP server stdout → proxy output)
	go func() {
		defer wg.Done()
		p.serverReader(ctx)
		readerDone <- struct{}{}
	}()

	// Goroutine 2: client → server (proxy input → real MCP server stdin)
	go func() {
		defer wg.Done()
		p.clientReader(ctx)
		readerDone <- struct{}{}
	}()

	// Either side ending terminates a proxy session. Closing both streams
	// guarantees the opposite reader is unblocked before waiting for it.
	select {
	case <-ctx.Done():
		slog.Default().With("component", "mcp_proxy").Info("proxy context cancelled")
	case <-readerDone:
		slog.Default().With("component", "mcp_proxy").Info("proxy reader exited")
	}

	cancel()
	if err := p.Close(); err != nil {
		slog.Default().With("component", "mcp_proxy").Warn("proxy close error", "error", err)
	}
	wg.Wait()

	return nil
}

// Close shuts down the proxy and its transport.
func (p *MCPProxy) Close() error {
	p.closeOnce.Do(func() {
		p.closeInput()
		p.closeErr = p.transport.Close()
	})
	return p.closeErr
}

func (p *MCPProxy) closeInput() {
	p.inputCloseOnce.Do(func() {
		_ = p.input.Close()
	})
}

// SessionID returns the proxy's session identifier.
func (p *MCPProxy) SessionID() string { return p.sessionID }

// ServerName returns the MCP server name discovered during initialize.
func (p *MCPProxy) ServerName() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.serverName
}

// AgentName returns the effective agent name.
func (p *MCPProxy) AgentName() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.agentName
}

// ServerTools returns the tools discovered from tools/list.
func (p *MCPProxy) ServerTools() []MCPTool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]MCPTool(nil), p.serverTools...)
}

// ---------------------------------------------------------------------------
// Reader goroutines
// ---------------------------------------------------------------------------

// serverReader reads messages from the real MCP server and forwards them
// to os.Stdout (the AI agent).  For tool call responses it records the
// completion event.
func (p *MCPProxy) serverReader(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := p.transport.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Default().With("component", "mcp_proxy", "direction", "server->client").Error("receive error", "error", err)
			return
		}

		// Match responses with the request that produced them before deriving
		// event or server metadata from their payload.
		if msg.IsResponse() {
			p.handleServerResponse(msg)
		}

		// Forward to client (os.Stdout).
		if err := p.writeToClient(msg); err != nil {
			slog.Default().With("component", "mcp_proxy", "direction", "server->client").Error("write to client", "error", err)
			return
		}
	}
}

// clientReader reads messages from the configured input (the AI agent) and forwards
// them to the real MCP server.  For tools/call requests it records the
// start event metadata.
func (p *MCPProxy) clientReader(ctx context.Context) {
	scanner := newInputScanner(p.input)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		data := scanner.Bytes()
		msg, err := ParseJSONRPCMessage(data)
		if err != nil {
			slog.Default().With("component", "mcp_proxy", "direction", "client->server").Warn("parse error", "error", err)
			// Forward the original bytes anyway — don't break the protocol.
			p.forwardRawToServer(data)
			continue
		}

		if msg.IsRequest() {
			switch msg.Method {
			case MCPMethodToolsCall:
				p.handleToolsCallStart(msg)
			case MCPMethodInitialize, MCPMethodToolsList:
				p.trackControlRequest(msg)
			}
		}

		// Forward to real server.
		if err := p.transport.Send(msg); err != nil {
			slog.Default().With("component", "mcp_proxy", "direction", "client->server").Error("send error", "error", err)
			return
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		slog.Default().With("component", "mcp_proxy", "direction", "client->server").Error("stdin read error", "error", err)
	}
}

// ---------------------------------------------------------------------------
// Interception handlers
// ---------------------------------------------------------------------------

// handleToolsCallStart records the start of a tools/call request.
func (p *MCPProxy) handleToolsCallStart(msg *JSONRPCMessage) {
	params, err := ParseToolCallParams(msg)
	if err != nil {
		slog.Default().With("component", "mcp_proxy").Warn("could not parse tools/call params", "error", err)
		return
	}

	reqSize := int64(len(msg.Params))

	p.mu.Lock()
	p.pending[msg.IDString()] = &pendingCall{
		toolName:    params.Name,
		startTime:   time.Now().UTC(),
		requestSize: reqSize,
	}
	p.mu.Unlock()

	slog.Default().With("component", "mcp_proxy", "tool", params.Name, "id", msg.IDString()).Debug("tools/call started")
}

func (p *MCPProxy) trackControlRequest(msg *JSONRPCMessage) {
	if id := msg.IDString(); id != "" {
		p.mu.Lock()
		p.pendingMethods[id] = msg.Method
		p.mu.Unlock()
	}
}

// handleServerResponse processes a response from the real MCP server.
// When it matches a pending tools/call, it builds and publishes a ToolEvent.
func (p *MCPProxy) handleServerResponse(msg *JSONRPCMessage) {
	id := msg.IDString()
	if id == "" {
		return
	}

	p.mu.Lock()
	call, ok := p.pending[id]
	if ok {
		delete(p.pending, id)
	}
	method := p.pendingMethods[id]
	delete(p.pendingMethods, id)
	p.mu.Unlock()

	if ok {
		p.publishToolCall(call, msg)
	}
	if msg.Result == nil {
		return
	}
	switch method {
	case MCPMethodInitialize:
		p.maybeCaptureInitialize(msg)
	case MCPMethodToolsList:
		p.maybeCaptureToolsList(msg)
	}
}

func (p *MCPProxy) publishToolCall(call *pendingCall, msg *JSONRPCMessage) {
	now := time.Now().UTC()
	durationMS := now.Sub(call.startTime).Milliseconds()

	resultSize := int64(len(msg.Result))
	errorSize := int64(0)
	if msg.Error != nil {
		errorSize = int64(len(msg.Error.Data))
	}

	success := !msg.IsError()

	event := events.NewToolEvent()
	event.Timestamp = call.startTime
	event.SessionID = p.sessionID
	event.AdapterName = "mcp"
	event.AdapterVersion = p.adapterVersion
	p.mu.Lock()
	event.AgentName = p.agentName
	serverName := p.serverName
	p.mu.Unlock()
	event.AgentVersion = "" // derived from server info if available
	event.ToolType = "mcp"
	event.ToolName = call.toolName
	event.FunctionName = "tools/call"
	event.Success = success
	event.DurationMS = durationMS
	event.InputSize = call.requestSize
	event.OutputSize = resultSize + errorSize

	if !success {
		event.ErrorType = ClassifyJSONRPCError(msg.Error.Code)
		event.ErrorCode = fmt.Sprintf("%d", msg.Error.Code)
		event.ErrorMessage = msg.Error.Message
	}

	// Attach MCP metadata.
	if event.Metadata == nil {
		event.Metadata = make(map[string]interface{})
	}
	event.Metadata["mcp_server"] = serverName
	event.Metadata["mcp_server_command"] = p.serverCommand

	// Publish through the standard collector for batching + retry.
	if err := p.collector.Publish(event); err != nil {
		slog.Default().With("component", "mcp_proxy", "event_id", event.EventID, "tool", call.toolName).Warn("tool event was not queued", "error", err)
	}

	slog.Default().With("component", "mcp_proxy",
		"tool", call.toolName,
		"success", success,
		"duration_ms", durationMS,
		"input_size", call.requestSize,
		"output_size", resultSize,
		"error_type", event.ErrorType,
		"event_id", event.EventID,
	).Info("tool event recorded")
}

// maybeCaptureInitialize extracts the server identity from an initialize
// response and updates the agent name if it was not explicitly set.
func (p *MCPProxy) maybeCaptureInitialize(msg *JSONRPCMessage) {
	result, err := ParseInitializeResult(msg)
	if err != nil {
		return // not an initialize response, or unexpected shape
	}

	if result.ServerInfo.Name == "" {
		return
	}

	p.mu.Lock()
	if p.serverName != "" {
		p.mu.Unlock()
		return // already captured
	}
	p.serverName = result.ServerInfo.Name
	if p.agentName == "" {
		// Derive agent name from the connected MCP server.
		p.agentName = "mcp-client:" + result.ServerInfo.Name
	}
	agentName := p.agentName
	p.mu.Unlock()

	slog.Default().With("component", "mcp_proxy",
		"server_name", result.ServerInfo.Name,
		"server_version", result.ServerInfo.Version,
		"protocol_version", result.ProtocolVersion,
		"agent_name", agentName,
	).Info("mcp initialize captured")
}

// commandLabel preserves enough information to identify the transport while
// deliberately excluding arguments, which often contain credentials.
func commandLabel(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return ""
	}
	return filepath.Base(fields[0])
}

// maybeCaptureToolsList records the tool catalog from a tools/list response.
func (p *MCPProxy) maybeCaptureToolsList(msg *JSONRPCMessage) {
	result, err := ParseToolsListResult(msg)
	if err != nil {
		return
	}

	p.mu.Lock()
	p.serverTools = result.Tools
	p.mu.Unlock()

	slog.Default().With("component", "mcp_proxy", "tool_count", len(result.Tools)).Info("tools/list captured")
}

// ---------------------------------------------------------------------------
// I/O helpers
// ---------------------------------------------------------------------------

// writeToClient writes a message to the configured output (newline-delimited JSON).
func (p *MCPProxy) writeToClient(msg *JSONRPCMessage) error {
	data, err := MarshalJSONRPCMessage(msg)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(p.output, "%s\n", data)
	return err
}

// forwardRawToServer sends raw bytes to the real MCP server without
// re-serialising.  Used as a fallback when we cannot parse a client message.
func (p *MCPProxy) forwardRawToServer(data []byte) {
	if err := p.transport.SendRaw(data); err != nil {
		slog.Default().With("component", "mcp_proxy", "direction", "client->server").Error("raw forward error", "error", err)
	}
}

// newInputScanner returns a buffered scanner reading from a proxy input.
// Uses a large buffer to handle big MCP messages.
func newInputScanner(input io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	return scanner
}
