# TraceAI

[English](docs/README.en.md)

> 面向 AI Agent 工具调用的本地可观测与产品分析工具。

TraceAI 记录 AI Agent 对外部工具的真实使用情况，并把不同接入方式统一成可以查询和分析的 `ToolEvent`：

- 哪个 Agent、Provider 或模型使用了工具。
- 使用了哪个工具和具体功能。
- 调用是否成功、失败原因是什么。
- 工具执行耗时、输入大小和输出大小。
- 哪些工具最常用、失败率最高、被反复重试或从未调用。

TraceAI 不是 Token 统计器，也不只服务于 MCP。项目目标是为 MCP Server、Function Calling、Agent Tool、HTTP/gRPC 服务和自定义 Agent 提供一套 local-first 的 AI-native product analytics 底座。

## 为什么需要 TraceAI

一个工具被安装、注册或暴露给模型，不代表 AI 真的会使用它。

TraceAI 希望回答工具开发者真正关心的问题：

```text
GitHub MCP
  search_code       1,280 calls   98.7% success
  get_file            820 calls   99.1% success
  create_pr            523 calls   91.2% success
  merge_pr              45 calls   62.3% success
  delete_branch          0 calls   zero usage
```

这些数据可以帮助开发者发现：

- 工具描述是否足够清晰，AI 是否知道什么时候调用。
- 参数结构是否过于复杂，导致参数解析或执行失败。
- 哪些功能值得继续投入，哪些功能长期无人使用。
- 不同 Agent 的工具选择、调用顺序和重试行为有什么差异。

## 当前能力

| 能力 | 状态 | 说明 |
| --- | --- | --- |
| MCP stdio 透明代理 | 可用 | 拦截 `tools/list` 和 `tools/call`，无需修改 MCP Server |
| Go SDK | 可用 | 支持手工记录、调用生命周期包装和自定义 Agent |
| HTTP 中间件 | 可用 | 记录请求状态、耗时和输入输出大小 |
| gRPC 拦截器 | 可用 | 支持 unary 和 stream server 调用 |
| CLI 分析与导出 | 可用 | 排行、趋势、错误、调用序列、重试模式、CSV/JSON |
| 本地 Web Dashboard | 可用 | 查看工具、Agent 和错误分析 |
| OpenAI 原生 Function Calling | 可用 | Responses / Chat Completions 非流式 Provider Capture + 真实执行关联 |
| Anthropic Tool Use | 可用 | Messages 非流式 Provider Capture + 真实执行关联 |
| OpenAI / Anthropic 流式采集 | 规划中 | 需要按 tool call ID 聚合流式参数片段 |
| npm / Python 注册表发布 | 准备中 | 保留 Go 核心，包管理器负责分发 CLI |

## 工作方式

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

TraceAI 有三种运行形态：

- 普通 CLI 命令按需启动，查询完成后退出。
- `traceai mcp proxy` 和 `traceai dashboard` 是持续运行的进程。
- Go SDK 嵌入用户应用，随应用生命周期持续记录。

单独拥有一个 `traceai.exe` 是正常的。它既包含命令行分析能力，也可以作为 MCP 代理或 Dashboard 服务长期运行。

## 快速开始

### 1. 安装

需要 Go 1.25 或更高版本。

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@latest
```

确保 Go 的 bin 目录已经加入 `PATH`，然后验证：

```bash
traceai version
traceai health
```

从源码构建：

```bash
git clone https://github.com/MIK-HEAL/TraceAI.git
cd TraceAI
go build -o bin/traceai ./cmd/traceai
```

Windows PowerShell：

```powershell
go build -o bin\traceai.exe ./cmd/traceai
.\bin\traceai.exe version
.\bin\traceai.exe health
```

PowerShell 默认不会执行当前目录中的裸命令。如果二进制就在当前目录，请使用 `.\traceai.exe`，或者把安装目录加入 `PATH`。

### 2. 体验分析结果

TraceAI 默认不会自动写入演示数据。需要试用时显式执行：

```bash
traceai --db traceai.db seed-demo
traceai --db traceai.db top-tools
traceai --db traceai.db stats
traceai --db traceai.db report --limit 5
```

启动本地 Dashboard：

```bash
traceai --db traceai.db dashboard --addr 127.0.0.1:8080
```

然后访问 `http://127.0.0.1:8080`。Dashboard 绑定非本机地址时必须配置 `--token`。

## 接入 MCP Server

MCP stdio 代理是目前最接近零代码的采集方式。TraceAI 启动真实 MCP Server，并在 Agent 与 Server 之间转发 JSON-RPC 消息。

原始命令：

```bash
npx -y @modelcontextprotocol/server-filesystem /path/to/workspace
```

接入 TraceAI：

```bash
traceai --db traceai.db mcp proxy \
  --mcp-cmd "npx -y @modelcontextprotocol/server-filesystem /path/to/workspace" \
  --agent-name my-agent
```

MCP 客户端配置示例：

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

代理的 stdout 专门用于 MCP JSON-RPC，运行日志写入 stderr，不会污染协议通信。

## 接入 Go SDK

对于自定义 Agent、HTTP、gRPC 或无法使用代理的场景，可以把 TraceAI 嵌入应用。

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

可用接入入口：

- `RecordStart` / `RecordFinish`：记录自定义调用生命周期。
- `HTTPMiddleware`：包装 `http.Handler`。
- `UnaryServerInterceptor` / `StreamServerInterceptor`：接入 gRPC Server。
- `WrapMCP`：包装 MCP tool handler。
- `CaptureRPC`：包装一次通用 RPC 或函数调用。

完整示例见 [examples](examples/) 和 [接入说明](docs/integration.md)。

## 原生 Function Calling

OpenAI 和 Anthropic 的模型响应只能证明“模型选择了工具”，不能证明工具已经真实执行。

TraceAI 的原生 Function Calling 方案采用两个采集点：

```text
Provider Adapter                     Tool Executor Wrapper
模型返回 tool call                   应用解析参数并执行函数
        |                                      |
        v                                      v
记录模型选择                           记录成功、错误和耗时
        +---------- tool_call_id -------------+
```

这样可以分别分析：

- 哪个 Provider、模型或 Agent 更常选择某个工具。
- 工具被选择后是否真的执行。
- 工具执行失败是参数、调度、超时还是业务错误。
- 模型选择工具与工具真实成功率之间的差异。

当前已实现 OpenAI Responses / Chat Completions 与 Anthropic Messages 的非流式 Provider Adapter。将 Provider-aware HTTP Transport 注入 SDK 的 HTTP Client，再将 registry.Take(toolCallID) 得到的上下文传给 Tool Executor Wrapper，即可在工具真正执行后记录成功、耗时和输入输出大小。完整示例见 [OpenAI](examples/openai/main.go)、[Anthropic](examples/anthropic/main.go) 和 [接入说明](docs/integration.md)。

## 分析命令

全局参数必须放在子命令前，例如 `traceai --db traceai.db report`。

| 命令 | 用途 |
| --- | --- |
| `traceai top-tools` | 工具调用排行 |
| `traceai top-functions` | 具体功能调用排行 |
| `traceai top-agents` | Agent 使用排行 |
| `traceai stats` | 总调用量、成功率、平均耗时和数据量 |
| `traceai report` | 工具热力图、失败排行、趋势和 Agent 报告 |
| `traceai call-seq --depth 2` | 常见工具调用序列 |
| `traceai retry-patterns` | 工具重试和恢复模式 |
| `traceai zero-calls --catalog ...` | 对照工具目录检测零调用能力 |
| `traceai high-failures --threshold 0.3` | 检测高失败率工具 |
| `traceai export top-tools --format csv` | 导出 CSV 或 JSON |
| `traceai dashboard` | 启动本地只读 Dashboard |
| `traceai status` | 查看存储和队列状态 |
| `traceai health` | 执行健康检查 |
| `traceai metrics --format json` | 输出运行和调用指标 |

导出示例：

```bash
traceai --db traceai.db export top-tools \
  --format csv \
  --output top-tools.csv
```

支持的导出目标：`top-tools`、`top-functions`、`top-agents`、`stats`、`daily-stats`、`weekly-stats` 和 `monthly-stats`。

## 配置

TraceAI 支持命令行参数、环境变量和 JSON 配置文件。

| 命令行参数 | 环境变量 | 默认值 |
| --- | --- | --- |
| `--store` | `TRACEAI_STORE` | `sqlite` |
| `--db` | `TRACEAI_DB` | `traceai.db` |
| `--config` | `TRACEAI_CONFIG` | 空 |
| `--log-level` | `TRACEAI_LOG_LEVEL` | `info` |
| `--log-format` | `TRACEAI_LOG_FORMAT` | `text` |

配置文件示例：

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

## 数据与隐私

- SQLite 数据默认保存在本地 `traceai.db`。
- 内置采集路径默认关注名称、状态、耗时和大小，不需要云端服务。
- MCP 代理不会把运行日志写到协议 stdout。
- SDK 使用者应避免把密钥、Prompt、完整参数或敏感结果放入自定义 metadata。
- TraceAI 当前不应作为唯一的生产监控或审计系统使用。

## 项目状态

TraceAI 当前处于开发预览阶段，适合：

- 本地试用和数据价值验证。
- MCP Server 与 Agent Tool 的接入测试。
- 工具调用行为、失败率和使用趋势分析。
- 为工具描述、参数 Schema 和功能规划提供数据反馈。

当前不建议直接将它作为关键生产系统的唯一遥测链路。正式生产化仍需要继续验证跨平台发布、长期运行、容量边界、升级迁移和原生 Function Calling 的完整覆盖。

npm 和 Python 分发包装器已经放在 `distribution/`，但在注册表发布完成前，推荐使用 Go 安装或从源码构建。

## 开发与验证

```bash
go test ./...
go build -o bin/traceai ./cmd/traceai
```

## 文档

- [英文 README](docs/README.en.md)
- [接入说明](docs/integration.md)
- [事件模型](docs/event-schema.md)
- [事件样例](docs/event-sample.json)
- [指标口径](docs/metrics.md)
- [存储结构](docs/storage-schema.md)
- [示例代码](examples/)

## 许可证

TraceAI 使用 [MIT License](LICENSE)。
