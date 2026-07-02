# TraceAI

> **Observable MCP Proxy & Tool Usage Analytics for AI Agents**

TraceAI 是一个**零侵入**的 MCP 透明代理和分析层。它拦截 AI Agent（Claude Code、Cursor 等）与 MCP Server 之间的每一次工具调用，回答三个核心问题：

- AI 实际在用哪些 MCP 工具？哪些从未被调用？
- 哪些工具频繁失败、让 AI 反复重试？
- 工具描述和参数设计是否对 AI 友好？

**不需要改 Agent 框架。不需要给 MCP Server 加埋点。只改一行配置。**

---

## 为什么 TraceAI 不一样

传统方案要求你在每个 Agent 框架里写 Adapter，或者让 MCP Server 自己上报。TraceAI 换个思路——**做 MCP 协议的透明中间人**：

```text
❌ 旧方案：Claude Code → (你要写代码埋点) → MCP Server
❌ 旧方案：MCP Server → (你要改 Server 加埋点) → 你的分析平台

✅ TraceAI：Claude Code → traceai proxy → MCP Server
                  ↑ 自动拦截、记录、分析
```

你只需要把 MCP 配置里的 `command` 指向 TraceAI：

```json
{
  "mcpServers": {
    "github": {
      "command": "traceai",
      "args": ["mcp", "proxy", "--mcp-cmd", "npx -y @modelcontextprotocol/server-github", "--db", "~/.traceai/trace.db"]
    }
  }
}
```

**Agent 不知道 TraceAI 存在。MCP Server 不知道 TraceAI 存在。一切照旧。但每次工具调用都被自动记录下来。**

---

## 装了 12 个 MCP 插件，AI 真的在用几个？

一周后，打开 TraceAI：

```text
$ traceai --db trace.db report

Tool Heatmap
────────────────────────────────────────
github__search_code        1,280 calls  98.7% success
github__get_file             820 calls  99.1% success
github__create_pr            523 calls  91.2% success
github__merge_pr              45 calls  62.3% success   ⚠️ 高失败率
github__delete_branch          0 calls  ——              零调用

Call Sequences (top paths)
────────────────────────────────────────
search_code -> get_file                   312
get_file -> search_code                   287
search_code -> get_file -> create_pr      156

Retry Patterns
────────────────────────────────────────
Tool               OK    FAIL  RECOV  DEGR  MIXED
github__merge_pr    28     17     12      0      5
github__search_code 1263   12      8      2      3
```

**一眼看出问题：**
- `delete_branch` 零调用 — 描述不够清晰？还是 AI 不倾向删除操作？
- `merge_pr` 36% 失败率 — 参数 schema 有问题？还是权限配置不对？
- AI 的行为模式是 `search → read → create`，典型的"先看再改"

---

## 快速开始

### 1. 安装

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@latest
```

或从源码构建：

```bash
git clone https://github.com/MIK-HEAL/TraceAI
cd TraceAI
go build -o traceai ./cmd/traceai
```

### 2. 启动透明代理

把你现有的 MCP Server 启动命令前面加上 `traceai mcp proxy --mcp-cmd`：

```bash
# 原始启动方式（无 TraceAI）
npx -y @modelcontextprotocol/server-filesystem /path/to/workspace

# 加上 TraceAI 透明代理
traceai --db ./trace.db mcp proxy --mcp-cmd "npx -y @modelcontextprotocol/server-filesystem /path/to/workspace"
```

### 3. 配置 Claude Code / Cursor

修改你的 MCP 客户端配置（以 Claude Code 的 `claude_desktop_config.json` 为例）：

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "traceai",
      "args": [
        "--db", "/home/user/.traceai/trace.db",
        "mcp", "proxy",
        "--mcp-cmd", "npx -y @modelcontextprotocol/server-filesystem /path/to/workspace"
      ]
    },
    "github": {
      "command": "traceai",
      "args": [
        "--db", "/home/user/.traceai/trace.db",
        "mcp", "proxy",
        "--mcp-cmd", "npx -y @modelcontextprotocol/server-github"
      ]
    }
  }
}
```

Cursor 的配置同理，在 `~/.cursor/mcp.json` 中修改。

**改完重启 Claude Code / Cursor，TraceAI 就开始工作了。零侵入。**

### 4. 查看分析结果

```bash
# 工具热力图
traceai --db trace.db top-tools

# 调用序列分析（AI 的行为路径）
traceai --db trace.db call-seq --depth 2
traceai --db trace.db call-seq --depth 3

# 重试模式分析（哪些工具让 AI 反复尝试）
traceai --db trace.db retry-patterns

# 零调用工具检测（需要提供预期工具目录）
traceai --db trace.db zero-calls --catalog "search_code,get_file,create_pr,merge_pr,delete_branch"

# 高失败率工具
traceai --db trace.db high-failures --threshold 0.3

# 完整报告
traceai --db trace.db report

# 启动 Web Dashboard
traceai --db trace.db dashboard --addr 127.0.0.1:8080
```

---

## 架构

```text
Claude Code / Cursor / LangChain
      │  (stdio JSON-RPC)
      ▼
┌──────────────────────┐
│  TraceAI MCP Proxy   │  ← 透明拦截 initialize / tools/list / tools/call
│  (stdio ↔ subprocess)│    记录 ToolEvent → EventBus → SQLite
└──────────────────────┘
      │  stdin/stdout
      ▼
  Real MCP Server       ← 完全无感知，代码一行不改
```

```text
┌──────────────────────────────────────────────────┐
│                  TraceAI 内部                      │
│                                                    │
│  MCP Proxy ──→ EventBus ──→ SQLite ──→ CLI        │
│  (adapters)    (collector)  (storage)   Dashboard  │
│                                                    │
│  所有 MCP 协议细节收敛在 internal/adapters/ 内     │
│  下游组件只消费统一 ToolEvent，不感知 MCP 协议     │
└──────────────────────────────────────────────────┘
```

### 统一事件模型

```json
{
  "event_id": "evt_abc123",
  "schema_version": "v1",
  "trace_id": "trc_xyz789",
  "session_id": "ses_def456",
  "timestamp": "2026-07-02T10:30:00Z",
  "agent_name": "mcp-client:github",
  "adapter_name": "mcp",
  "adapter_version": "0.2.0",
  "tool_type": "mcp",
  "tool_name": "search_code",
  "function_name": "tools/call",
  "success": true,
  "duration_ms": 245,
  "input_size": 1024,
  "output_size": 8192,
  "retry_count": 0,
  "error_type": "",
  "error_message": "",
  "metadata": {
    "mcp_server": "github",
    "mcp_server_cmd": "npx -y @modelcontextprotocol/server-github"
  }
}
```

---

## CLI 命令参考

| 命令 | 说明 |
|------|------|
| `traceai mcp proxy --mcp-cmd "..."` | 启动 MCP 透明代理 |
| `traceai top-tools` | 工具调用排行榜 |
| `traceai top-functions` | 函数调用排行榜 |
| `traceai top-agents` | Agent 使用排行榜 |
| `traceai call-seq [--depth 2\|3]` | 调用序列分析 |
| `traceai retry-patterns` | 重试模式分析 |
| `traceai zero-calls --catalog ...` | 零调用工具检测 |
| `traceai high-failures [--threshold 0.3]` | 高失败率工具 |
| `traceai stats` | 总体统计 |
| `traceai report` | 完整分析报告 |
| `traceai dashboard` | 启动 Web Dashboard |
| `traceai export <target>` | 导出 CSV / JSON |
| `traceai status` | 运行状态 |
| `traceai health` | 健康检查 |
| `traceai metrics` | 指标输出 |
| `traceai seed-demo` | 写入演示数据 |

---

## 附加集成（非主线）

对于非 MCP 场景（OpenAI Function Calling、gRPC、HTTP API），TraceAI 也提供手动埋点 Adapter：

```go
// HTTP 中间件
handler := traceai.HTTPMiddleware(client, traceai.CallInfo{
    AdapterName: "http",
    ToolName:    "health",
})(yourHandler)

// gRPC 拦截器
traceai.UnaryServerInterceptor(client, traceai.CallInfo{...})

// 手工埋点
ctx = traceai.RecordStart(ctx, client, info)
err := doSomething(ctx)
traceai.RecordFinish(ctx, err == nil, inputSize, outputSize, err)
```

详见 [接入说明](docs/integration.md) 和 [示例代码](examples/)。

---

## 项目状态

**v0.2（当前）：**
- [x] MCP stdio 透明代理（零侵入接入）
- [x] 调用序列分析
- [x] 重试模式分析
- [x] 零调用 / 高失败率工具检测
- [x] MCP 协议细节收敛在 Adapter 层

**之前版本已完成的：**
- 统一事件模型、SQLite 存储、事件总线
- CLI 分析命令、Web Dashboard
- OTLP 导出、结构化日志
- npm / pip 分发

---

## 文档

- [快速开始](docs/quickstart.md)
- [事件模型](docs/event-schema.md)
- [事件样例](docs/event-sample.json)
- [接入说明](docs/integration.md)
- [指标口径](docs/metrics.md)
- [存储结构](docs/storage-schema.md)

---

## 许可证

MIT，见 [LICENSE](LICENSE)
