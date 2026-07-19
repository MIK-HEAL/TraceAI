package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/adapters"
	"github.com/MIK-HEAL/TraceAI/internal/collector"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

// runMCP handles the `traceai mcp` subcommand tree.
func runMCP(store storage.Storage, args []string, out io.Writer) error {
	if len(args) < 1 {
		fmt.Fprintln(out, "Usage: traceai mcp proxy --mcp-cmd \"command...\" [--agent-name name]")
		return nil
	}

	switch args[0] {
	case "proxy":
		return runMCPProxy(store, args[1:], out)
	default:
		fmt.Fprintf(out, "unknown mcp subcommand: %q\n", args[0])
		fmt.Fprintln(out, "Usage: traceai mcp proxy --mcp-cmd \"command...\" [--agent-name name]")
		return nil
	}
}

// runMCPProxy starts the transparent MCP proxy.
func runMCPProxy(store storage.Storage, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("mcp proxy", flag.ExitOnError)
	mcpCmd := fs.String("mcp-cmd", "", "command to launch the real MCP server (required)")
	agentName := fs.String("agent-name", "", "override agent name in recorded events")
	_ = fs.Parse(args)

	if *mcpCmd == "" {
		fs.Usage()
		return errors.New("--mcp-cmd is required")
	}

	// Build proxy configuration.
	cfg := &adapters.MCPProxyConfig{
		MCPCmd:    *mcpCmd,
		AgentName: *agentName,
	}

	// Create collector — wraps the event bus for async batched writes.
	col := collector.NewCollector(store)

	// Build the proxy.
	proxy, err := adapters.NewMCPProxy(cfg, col)
	if err != nil {
		return fmt.Errorf("create mcp proxy: %w", err)
	}

	// Signal-aware context for graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start the collector (background event bus worker).
	if err := col.Start(ctx); err != nil {
		return fmt.Errorf("start collector: %w", err)
	}

	// Write status to stderr — stdout is reserved for MCP JSON-RPC protocol.
	_, _ = fmt.Fprintf(os.Stderr, "TraceAI MCP Proxy starting\n")
	_, _ = fmt.Fprintf(os.Stderr, "  session: %s\n", proxy.SessionID())
	_, _ = fmt.Fprintln(os.Stderr, "  command: configured (arguments redacted)")
	_, _ = fmt.Fprintf(os.Stderr, "  agent:   %s\n", proxy.AgentName())
	_, _ = fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop.\n\n")

	// Run the proxy (blocks until interrupted).
	if err := proxy.Run(ctx); err != nil {
		slog.Default().With("component", "mcp_proxy_cli").Error("proxy run error", "error", err)
	}

	// Graceful shutdown (status to stderr — stdout is protocol).
	_, _ = fmt.Fprintf(os.Stderr, "\nShutting down...\n")

	if err := proxy.Close(); err != nil {
		slog.Default().With("component", "mcp_proxy_cli").Warn("proxy close error", "error", err)
	}

	if err := col.Close(5 * time.Second); err != nil {
		slog.Default().With("component", "mcp_proxy_cli").Warn("collector close error", "error", err)
	}

	_, _ = fmt.Fprintf(os.Stderr, "Proxy stopped.\n")
	return nil
}
