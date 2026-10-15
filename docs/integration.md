# TraceAI 接入说明

## 推荐入口

优先使用 `pkg/traceai`：

- `traceai.New(traceai.NewMemoryStore())`
- `traceai.Interceptor`
- `traceai.SemanticFields()`
- `traceai.NewLocalExporter()`
- `traceai.NewOTLPExporter()`

## 最小流程

1. 初始化 `Client`
2. `Start(ctx)` 启动存储
3. 用 `Interceptor` 包装 HTTP / gRPC / MCP 调用
4. 通过 `Publish(...)` 或自动拦截写入事件
5. 用 `TopTools / Stats / DailyStats / ErrorBreakdowns` 查询结果
6. `Close(timeout)` 收尾

## 语义字段

TraceAI 统一使用这些字段：

- `traceai.tool.name`
- `traceai.tool.type`
- `traceai.tool.success`
- `traceai.tool.duration_ms`
- `traceai.agent.name`
- `traceai.error.code`
- `traceai.error.type`

## 自动拦截

### HTTP

使用 `WrapHTTP` 包装 `http.Handler`。

### gRPC

使用 `CaptureRPC` 包装一次 RPC 调用。

### MCP

使用 `WrapMCP` 包装 tool handler。

## 原生 Function Calling

OpenAI 和 Anthropic Provider Adapter 只提取模型选择的函数调用上下文；真正的 ToolEvent 只会在 tool.Wrap 包装的函数执行结束后写入。因此，模型请求失败、解析失败或“选择后未执行”都不会污染工具成功率。

~~~go
registry := provider.NewRegistry()
modelClient := &http.Client{
    Transport: httptransport.NewTransport(
        http.DefaultTransport,
        registry,
        openai.NewAdapter(openai.Config{
            ToolNamespaces: map[string]string{"create_issue": "github"},
        }),
    ),
}

// Pass modelClient to the Provider SDK. After receiving callID from the model:
decision, ok := registry.Take(callID)
if !ok {
    return errors.New("missing provider decision")
}
handler := tool.Wrap(decision, telemetry, createIssue)
result, err := handler(ctx, input)
~~~

Provider metadata stores no Prompt, API Key, original arguments, or tool result:

- traceai.provider.name
- traceai.model.name
- traceai.api.family
- traceai.request.id
- traceai.tool.call_id
- traceai.tool.parent_event_id

OpenAI Responses / Chat Completions and Anthropic Messages support both regular and
SSE streaming responses. Stream fragments are grouped by provider tool-call ID and
only sizes and stable identifiers are persisted; argument contents remain subject
to the configured capture mode.

## 导出

- `LocalExporter` 输出 JSONL 到系统临时目录
- `OTLPExporter` 输出 OTLP 映射后的 JSONL 到系统临时目录

## 示例

- [HTTP 示例](../examples/http/main.go)
- [gRPC 示例](../examples/grpc/main.go)
- [MCP 示例](../examples/mcp/main.go)
- [OpenAI 示例](../examples/openai/main.go)
- [Anthropic 示例](../examples/anthropic/main.go)
# 10 分钟首次接入

1. 安装 `@clonap4d/traceai`（或 Python wrapper），确认 `traceai version` 可运行。
2. 用 `traceai mcp proxy --mcp-cmd "<your server command>"` 替换 MCP client 的 server command。
3. 运行一次 `tools/list` 和一个 `tools/call`，再执行 `traceai report --catalog <tool-catalog>`。
4. 在 `traceai dashboard` 的 Tools 页面查看调用率、成功率、重试、平均耗时和零调用工具。

默认使用 `redacted` 采集模式；真实环境可按需设置 `--capture-mode metadata-only`、
`--retention-days` 和 `--max-db-bytes`。
