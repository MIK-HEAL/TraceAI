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
// All transports produce and consume JSONRPCMessage values; the proxy operates
// on messages without knowing whether they arrived via stdio, SSE, or HTTP.
type MCPTransport interface {
	Start(ctx context.Context) error
	Send(msg *JSONRPCMessage) error
	SendRaw(data []byte) error
	Receive(ctx context.Context) (*JSONRPCMessage, error)
	Close() error
}

type transportResult struct {
	msg *JSONRPCMessage
	err error
}

// StdioTransport launches an MCP child process and communicates over its
// newline-delimited JSON stdin/stdout streams.
type StdioTransport struct {
	serverCommand string
	cmd           *exec.Cmd

	stateMu   sync.Mutex
	writeMu   sync.Mutex
	closeOnce sync.Once
	started   bool
	stopped   bool
	stdin     io.WriteCloser
	stderr    io.ReadCloser

	results    chan transportResult
	closed     chan struct{}
	readDone   chan struct{}
	stderrDone chan struct{}
	closeErr   error
}

// NewStdioTransport creates a transport that will launch the given command.
// The command is split on whitespace; use a wrapper script for arguments that
// themselves contain spaces.
func NewStdioTransport(mcpCmd string) (*StdioTransport, error) {
	parts := strings.Fields(strings.TrimSpace(mcpCmd))
	if len(parts) == 0 {
		return nil, fmt.Errorf("mcp command must not be empty")
	}
	return &StdioTransport{
		serverCommand: filepath.Base(parts[0]),
		cmd:           exec.Command(parts[0], parts[1:]...),
	}, nil
}

// Start launches the child process and starts exactly one stdout reader. The
// reader outlives individual Receive contexts, avoiding concurrent calls to a
// bufio.Scanner when callers cancel a receive.
func (t *StdioTransport) Start(ctx context.Context) error {
	_ = ctx
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.started {
		return fmt.Errorf("transport already started")
	}
	if t.stopped {
		return fmt.Errorf("transport is closed")
	}

	stdin, err := t.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := t.cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := t.cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := t.cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("start mcp server %q: %w", t.serverCommand, err)
	}

	t.stdin = stdin
	t.stderr = stderr
	t.results = make(chan transportResult, 16)
	t.closed = make(chan struct{})
	t.readDone = make(chan struct{})
	t.stderrDone = make(chan struct{})
	t.started = true

	go t.readStdout(stdout, t.results, t.closed, t.readDone)
	go t.drainStderr(stderr, t.closed, t.stderrDone)
	slog.Default().With("component", "mcp_proxy", "server", t.serverCommand, "pid", t.cmd.Process.Pid).Info("mcp server started")
	return nil
}

func (t *StdioTransport) readStdout(stdout io.ReadCloser, results chan<- transportResult, closed <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	defer close(results)
	defer stdout.Close()

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		data := append([]byte(nil), scanner.Bytes()...)
		msg, err := ParseJSONRPCMessage(data)
		if err != nil {
			t.sendResult(results, closed, transportResult{err: err})
			return
		}
		if !t.sendResult(results, closed, transportResult{msg: msg}) {
			return
		}
	}
	if err := scanner.Err(); err != nil {
		t.sendResult(results, closed, transportResult{err: fmt.Errorf("read from mcp server: %w", err)})
		return
	}
	t.sendResult(results, closed, transportResult{err: io.EOF})
}

func (t *StdioTransport) sendResult(results chan<- transportResult, closed <-chan struct{}, result transportResult) bool {
	select {
	case results <- result:
		return true
	case <-closed:
		return false
	}
}

func (t *StdioTransport) drainStderr(stderr io.Reader, closed <-chan struct{}, done chan<- struct{}) {
	defer close(done)
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		select {
		case <-closed:
			return
		default:
			slog.Default().With("component", "mcp_proxy", "server", t.serverCommand).Info("mcp_server_stderr", "msg", scanner.Text())
		}
	}
}

// Send writes a JSON-RPC message to the child process stdin.
func (t *StdioTransport) Send(msg *JSONRPCMessage) error {
	data, err := MarshalJSONRPCMessage(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	return t.writeLine(data)
}

// SendRaw writes raw bytes to the child process stdin when a client message
// cannot be parsed but must still be forwarded.
func (t *StdioTransport) SendRaw(data []byte) error {
	return t.writeLine(data)
}

func (t *StdioTransport) writeLine(data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	t.stateMu.Lock()
	started, stdin := t.started, t.stdin
	t.stateMu.Unlock()
	if !started || stdin == nil {
		return fmt.Errorf("transport not started")
	}
	if _, err := fmt.Fprintf(stdin, "%s\n", data); err != nil {
		return fmt.Errorf("write to mcp server: %w", err)
	}
	return nil
}

// Receive waits for the single stdout reader or returns when ctx is cancelled.
func (t *StdioTransport) Receive(ctx context.Context) (*JSONRPCMessage, error) {
	t.stateMu.Lock()
	if !t.started || t.results == nil {
		t.stateMu.Unlock()
		return nil, fmt.Errorf("transport not started")
	}
	results := t.results
	t.stateMu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result, ok := <-results:
		if !ok {
			return nil, io.EOF
		}
		return result.msg, result.err
	}
}

// Close terminates the child, unblocks the stdout reader, and waits for that
// reader to finish. It is safe to call multiple times.
func (t *StdioTransport) Close() error {
	t.closeOnce.Do(func() {
		t.stateMu.Lock()
		t.stopped = true
		if !t.started {
			t.stateMu.Unlock()
			return
		}
		t.started = false
		stdin, cmd, closed, readDone, stderrDone := t.stdin, t.cmd, t.closed, t.readDone, t.stderrDone
		t.stateMu.Unlock()

		close(closed)
		if stdin != nil {
			_ = stdin.Close()
		}
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			if err := cmd.Wait(); err != nil {
				if _, killed := err.(*exec.ExitError); !killed {
					t.closeErr = fmt.Errorf("close transport: %w", err)
				}
			}
		}
		if readDone != nil {
			<-readDone
		}
		if stderrDone != nil {
			<-stderrDone
		}
		slog.Default().With("component", "mcp_proxy", "server", t.serverCommand).Info("mcp transport closed")
	})
	return t.closeErr
}

// Command returns a safe executable label for the configured command.
func (t *StdioTransport) Command() string {
	return t.serverCommand
}

// Ensure the JSON dependency remains part of the transport's supported
// protocol surface.
var _ = json.Marshal
