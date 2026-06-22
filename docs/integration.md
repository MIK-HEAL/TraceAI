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

## 导出

- `LocalExporter` 输出 JSONL 到系统临时目录
- `OTLPExporter` 预留外部监控系统接入位

## 示例

- [HTTP 示例](../examples/http/main.go)
- [gRPC 示例](../examples/grpc/main.go)
- [MCP 示例](../examples/mcp/main.go)
- [OpenAI 示例](../examples/openai/main.go)
