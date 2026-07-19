package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// MCPTransport is the abstraction over different MCP transport layers.
// All transports produce/consume JSONRPCMessage values; the proxy
// operates on messages without knowing whether they arrived via
// stdio, SSE, or HTTP.
type MCPTransport interface {
	// Start initialises the transport and makes it ready for Send/Receive.
	Start(ctx context.Context) error

	// Send writes a JSON-RPC message to the transport.
	Send(msg *JSONRPCMessage) error

	// SendRaw writes raw bytes to the transport (used as a fallback when
	// a message cannot be parsed as JSON-RPC but must still be forwarded).
	SendRaw(data []byte) error

	// Receive reads the next JSON-RPC message from the transport.
	// Must block until a message is available or the context is cancelled.
	Receive(ctx context.Context) (*JSONRPCMessage, error)

	// Close shuts down the transport and releases resources.
	Close() error
}

// ---------------------------------------------------------------------------
// Stdio transport
// ---------------------------------------------------------------------------

// StdioTransport implements MCPTransport by spawning a child process and
// communicating over its stdin/stdout with newline-delimited JSON.
type StdioTransport struct {
	serverCommand string
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	stdout        io.ReadCloser
	stderr        io.ReadCloser

	scanner *bufio.Scanner
	writeMu sync.Mutex
	readMu  sync.Mutex
	started bool
}

// NewStdioTransport creates a transport that will launch the given command.
// The command string is split on whitespace; for commands containing spaces
// in arguments, wrap the argument in a shell script or batch file.
func NewStdioTransport(mcpCmd string) (*StdioTransport, error) {
	mcpCmd = strings.TrimSpace(mcpCmd)
	if mcpCmd == "" {
		return nil, fmt.Errorf("mcp command must not be empty")
	}

	parts := strings.Fields(mcpCmd)
	if len(parts) == 0 {
		return nil, fmt.Errorf("mcp command must not be empty")
	}

	exe := parts[0]
	var args []string
	if len(parts) > 1 {
		args = parts[1:]
	}

	cmd := exec.Command(exe, args...)

	return &StdioTransport{serverCommand: filepath.Base(exe), cmd: cmd}, nil
}

// Start launches the child process and wires up stdin/stdout.
func (t *StdioTransport) Start(ctx context.Context) error {
	if t.started {
		return fmt.Errorf("transport already started")
	}

	var err error
	t.stdin, err = t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}

	t.stdout, err = t.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}

	t.stderr, err = t.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := t.cmd.Start(); err != nil {
		return fmt.Errorf("start mcp server %q: %w", t.serverCommand, err)
	}

	// Drain stderr in the background — MCP servers log diagnostics to stderr.
	go func() {
		scanner := bufio.NewScanner(t.stderr)
		for scanner.Scan() {
			slog.Default().With("component", "mcp_proxy", "server", t.serverCommand).Info("mcp_server_stderr", "msg", scanner.Text())
		}
	}()

	t.scanner = bufio.NewScanner(t.stdout)
	// MCP messages can be large (tool results). Use a generous buffer.
	t.scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	t.started = true
	slog.Default().With("component", "mcp_proxy", "server", t.serverCommand, "pid", t.cmd.Process.Pid).Info("mcp server started")
	return nil
}

// Send writes a JSON-RPC message to the child process stdin.
// Messages are newline-delimited JSON.
func (t *StdioTransport) Send(msg *JSONRPCMessage) error {
	data, err := MarshalJSONRPCMessage(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return t.writeLine(data)
}

// SendRaw writes raw bytes to the child process stdin.
// Used as a fallback when a client message cannot be parsed as JSON-RPC
// but must still be forwarded to the real server.
func (t *StdioTransport) SendRaw(data []byte) error {
	return t.writeLine(data)
}

// writeLine writes data followed by a newline to stdin.
func (t *StdioTransport) writeLine(data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()

	if !t.started {
		return fmt.Errorf("transport not started")
	}

	if _, err := fmt.Fprintf(t.stdin, "%s\n", data); err != nil {
		return fmt.Errorf("write to mcp server: %w", err)
	}
	return nil
}

// Receive reads the next JSON-RPC message from the child process stdout.
// Blocks until a message is available or ctx is cancelled.
func (t *StdioTransport) Receive(ctx context.Context) (*JSONRPCMessage, error) {
	t.readMu.Lock()
	defer t.readMu.Unlock()

	if !t.started {
		return nil, fmt.Errorf("transport not started")
	}

	// Use a channel to bridge the blocking scanner to context cancellation.
	type result struct {
		msg *JSONRPCMessage
		err error
	}
	ch := make(chan result, 1)

	go func() {
		if !t.scanner.Scan() {
			if err := t.scanner.Err(); err != nil {
				ch <- result{err: fmt.Errorf("read from mcp server: %w", err)}
			} else {
				ch <- result{err: io.EOF}
			}
			return
		}

		data := t.scanner.Bytes()
		// Make a copy — scanner reuses its buffer.
		buf := make([]byte, len(data))
		copy(buf, data)

		msg, err := ParseJSONRPCMessage(buf)
		if err != nil {
			ch <- result{err: err}
			return
		}
		ch <- result{msg: msg}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-ch:
		return res.msg, res.err
	}
}

// Close terminates the child process and releases resources.
func (t *StdioTransport) Close() error {
	if !t.started {
		return nil
	}
	t.started = false

	var errs []error

	// Close stdin to signal EOF to the child.
	if t.stdin != nil {
		_ = t.stdin.Close()
	}

	// Give the process a moment to exit gracefully, then kill.
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		if waitErr := t.cmd.Wait(); waitErr != nil {
			// Ignore "exit status" errors from killed processes.
			if _, ok := waitErr.(*exec.ExitError); !ok {
				errs = append(errs, waitErr)
			}
		}
	}

	slog.Default().With("component", "mcp_proxy", "server", t.serverCommand).Info("mcp transport closed")

	if len(errs) > 0 {
		return fmt.Errorf("close transport: %v", errs)
	}
	return nil
}

// Command returns a safe executable label for the configured command.
func (t *StdioTransport) Command() string {
	return t.serverCommand
}

// ensure json is available (used in the proxy).
var _ = json.Marshal
