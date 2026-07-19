# TraceAI

[简体中文](../README.md)

> Local-first observability and product analytics for AI agent tool calls.

TraceAI records how AI agents actually use external tools and normalizes every supported integration into analyzable `ToolEvent` data:

- Which agent, provider, or model used a tool.
- Which tool and specific function was called.
- Whether execution succeeded and why it failed.
- Execution latency, input size, and output size.
- Which tools are popular, error-prone, repeatedly retried, or never used.

TraceAI is not a token counter, and it is not limited to MCP. Its goal is to provide a local-first AI-native product analytics foundation for MCP servers, native function calling, agent tools, HTTP/gRPC services, and custom agents.

## Why TraceAI

Installing, registering, or exposing a tool to a model does not mean that an AI agent will actually use it.

TraceAI is designed to answer the questions tool developers care about:

```text
GitHub MCP
  search_code       1,280 calls   98.7% success
  get_file            820 calls   99.1% success
  create_pr            523 calls   91.2% success
  merge_pr              45 calls   62.3% success
  delete_branch          0 calls   zero usage
```

This data helps developers understand:

- Whether tool descriptions clearly tell the model when to call them.
- Whether parameter schemas are causing argument or execution failures.
- Which capabilities deserve more investment and which remain unused.
- How tool selection, call sequences, and retries differ across agents.

## Current Capabilities

| Capability | Status | Notes |
| --- | --- | --- |
| MCP stdio transparent proxy | Available | Observes `tools/list` and `tools/call` without modifying the MCP server |
| Go SDK | Available | Manual recording, lifecycle wrappers, and custom-agent integration |
| HTTP middleware | Available | Captures request status, latency, input size, and output size |
| gRPC interceptors | Available | Supports unary and stream server calls |
| CLI analytics and export | Available | Rankings, trends, errors, call sequences, retries, CSV, and JSON |
| Local web dashboard | Available | Tool, agent, and error analysis |
| Native OpenAI function calling | In development | Links provider decisions with actual tool execution |
| Anthropic tool use | In development | Initially targets non-streaming Messages calls |
| OpenAI / Anthropic streaming capture | Planned | Requires tool-call-aware aggregation of streamed argument fragments |
| npm / Python registry releases | In preparation | Go remains the core; package managers distribute the CLI |

## How It Works

```text
MCP Proxy / SDK / Middleware / Provider Adapter
                     |
                     v
                 ToolEvent
                     |
                     v
          Collector -> SQLite
                     |
                     v
       CLI / Dashboard / Export / Report
```

TraceAI has three runtime modes:

- Regular CLI commands start on demand and exit after completing a query.
- `traceai mcp proxy` and `traceai dashboard` are long-running processes.
- The Go SDK is embedded in an application and records data for the application's lifetime.

A single `traceai` executable is expected. It contains both one-shot analytics commands and long-running MCP proxy and dashboard modes.

## Quick Start

### 1. Install

Go 1.25 or later is required.

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@latest
```

Make sure the Go bin directory is on `PATH`, then verify the installation:

```bash
traceai version
traceai health
```

Build from source:

```bash
git clone https://github.com/MIK-HEAL/TraceAI.git
cd TraceAI
go build -o bin/traceai ./cmd/traceai
```

Windows PowerShell:

```powershell
go build -o bin\traceai.exe ./cmd/traceai
.\bin\traceai.exe version
.\bin\traceai.exe health
```

PowerShell does not execute bare commands from the current directory by default. Use `.\traceai.exe` when the binary is in the current directory, or add its installation directory to `PATH`.

### 2. Try the Analytics

TraceAI does not insert demo data automatically. Seed it explicitly when you want a quick trial:

```bash
traceai --db traceai.db seed-demo
traceai --db traceai.db top-tools
traceai --db traceai.db stats
traceai --db traceai.db report --limit 5
```

Start the local dashboard:

```bash
traceai --db traceai.db dashboard --addr 127.0.0.1:8080
```

Open `http://127.0.0.1:8080`. A `--token` is required when the dashboard binds to a non-loopback address.

## Connect an MCP Server

The MCP stdio proxy is currently the lowest-code capture path. TraceAI launches the real MCP server and forwards JSON-RPC messages between the agent and server.

Original command:

```bash
npx -y @modelcontextprotocol/server-filesystem /path/to/workspace
```

With TraceAI:

```bash
traceai --db traceai.db mcp proxy \
  --mcp-cmd "npx -y @modelcontextprotocol/server-filesystem /path/to/workspace" \
  --agent-name my-agent
```

Example MCP client configuration:

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "traceai",
      "args": [
        "--db",
        "/absolute/path/to/traceai.db",
        "mcp",
        "proxy",
        "--mcp-cmd",
        "npx -y @modelcontextprotocol/server-filesystem /path/to/workspace",
        "--agent-name",
        "my-agent"
      ]
    }
  }
}
```

Proxy stdout is reserved for MCP JSON-RPC. Runtime logs are written to stderr and do not corrupt protocol traffic.

## Use the Go SDK

Embed TraceAI when integrating a custom agent, HTTP service, gRPC service, or any environment where a proxy is not appropriate.

```go
package main

import (
    "context"
    "net/http"
    "time"

    "github.com/MIK-HEAL/TraceAI/pkg/traceai"
)

func main() {
    ctx := context.Background()
    client := traceai.New(traceai.OpenSQLite("traceai.db"))
    if err := client.Start(ctx); err != nil {
        panic(err)
    }
    defer client.Close(5 * time.Second)

    handler := traceai.HTTPMiddleware(client, traceai.CallInfo{
        AdapterName:  "http",
        AgentName:    "support-agent",
        ToolType:     "http",
        ToolName:     "customer-api",
        FunctionName: "GET /customers/:id",
    })(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, _ = w.Write([]byte("ok"))
    }))

    _ = http.ListenAndServe("127.0.0.1:8081", handler)
}
```

Available integration APIs:

- `RecordStart` / `RecordFinish` for custom call lifecycles.
- `HTTPMiddleware` for `http.Handler` instrumentation.
- `UnaryServerInterceptor` / `StreamServerInterceptor` for gRPC servers.
- `WrapMCP` for MCP tool handlers.
- `CaptureRPC` for generic RPC or function calls.

See [examples](../examples/) and the [integration guide](integration.md).

## Native Function Calling

An OpenAI or Anthropic model response proves that the model selected a tool. It does not prove that the application actually executed that tool.

TraceAI therefore uses two capture points:

```text
Provider Adapter                     Tool Executor Wrapper
model returns a tool call            application parses and executes it
        |                                      |
        v                                      v
record model decision               record success, error, and latency
        +---------- tool_call_id -------------+
```

This makes it possible to analyze:

- Which provider, model, or agent selects a tool most often.
- Whether a selected tool is actually executed.
- Whether execution fails during dispatch, argument parsing, timeout, or business logic.
- The difference between model selection rate and real execution success rate.

The first implementation targets non-streaming OpenAI and Anthropic provider adapters. Streaming aggregation and cross-language proxy capture will follow later.

## Analytics Commands

Global flags must appear before the subcommand, for example `traceai --db traceai.db report`.

| Command | Purpose |
| --- | --- |
| `traceai top-tools` | Rank tools by usage |
| `traceai top-functions` | Rank individual functions |
| `traceai top-agents` | Rank agents by usage |
| `traceai stats` | Calls, success rate, average latency, and data volume |
| `traceai report` | Tool heatmap, failures, trends, and agent report |
| `traceai call-seq --depth 2` | Common tool call sequences |
| `traceai retry-patterns` | Retry and recovery patterns |
| `traceai zero-calls --catalog ...` | Detect unused capabilities against a tool catalog |
| `traceai high-failures --threshold 0.3` | Detect high-failure tools |
| `traceai export top-tools --format csv` | Export CSV or JSON |
| `traceai dashboard` | Start the local read-only dashboard |
| `traceai status` | Inspect storage and queue state |
| `traceai health` | Run a health check |
| `traceai metrics --format json` | Print runtime and call metrics |

Export example:

```bash
traceai --db traceai.db export top-tools \
  --format csv \
  --output top-tools.csv
```

Supported export targets are `top-tools`, `top-functions`, `top-agents`, `stats`, `daily-stats`, `weekly-stats`, and `monthly-stats`.

## Configuration

TraceAI accepts command-line flags, environment variables, and a JSON configuration file.

| CLI flag | Environment variable | Default |
| --- | --- | --- |
| `--store` | `TRACEAI_STORE` | `sqlite` |
| `--db` | `TRACEAI_DB` | `traceai.db` |
| `--config` | `TRACEAI_CONFIG` | empty |
| `--log-level` | `TRACEAI_LOG_LEVEL` | `info` |
| `--log-format` | `TRACEAI_LOG_FORMAT` | `text` |

Example configuration file:

```json
{
  "Store": "sqlite",
  "DB": "traceai.db",
  "LogLevel": "info",
  "LogFormat": "text"
}
```

```bash
traceai --config traceai.json report
```

## Data and Privacy

- SQLite data is stored locally in `traceai.db` by default.
- Built-in capture paths focus on names, status, latency, and size without requiring a cloud service.
- The MCP proxy never writes runtime logs to protocol stdout.
- SDK users should avoid placing credentials, prompts, full arguments, or sensitive results in custom metadata.
- TraceAI should not currently be used as the only production monitoring or auditing system.

## Project Status

TraceAI is currently a development preview suitable for:

- Local trials and data-value validation.
- MCP server and agent-tool integration testing.
- Tool behavior, failure-rate, and usage-trend analysis.
- Improving tool descriptions, parameter schemas, and product planning with real usage data.

It is not yet recommended as the only telemetry path for a critical production system. Production readiness still requires more cross-platform release validation, long-running capacity testing, upgrade migration work, and complete native function-calling coverage.

npm and Python distribution wrappers are present under `distribution/`. Until registry publication is complete, Go installation or a source build is recommended.

## Development

```bash
go test ./...
go build -o bin/traceai ./cmd/traceai
```

## Documentation

- [Chinese README](../README.md)
- [Integration guide](integration.md)
- [Event schema](event-schema.md)
- [Event example](event-sample.json)
- [Metrics](metrics.md)
- [Storage schema](storage-schema.md)
- [Examples](../examples/)

## License

TraceAI is available under the [MIT License](../LICENSE).
