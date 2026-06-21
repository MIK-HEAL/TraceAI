# TraceAI

> **MCP Analytics — 回答一个让所有工具开发者夜不能寐的问题：我开发的工具，AI 真的在用吗？**

TraceAI 是一个面向 MCP Server、Agent Tool、插件开发者的 AI-Native 产品分析层。它记录 AI Agent 对工具的每一次调用，并回答：

- 哪些功能最常用？哪些功能几乎没人用？
- 哪些 API 设计让 AI 频繁失败？
- AI 的行为模式是什么？它在绕路还是走捷径？
- 你的工具对 AI 有多友好？

---

## 不是"统计工具调用次数"

传统思路是统计 Token 消耗或工具调用总量。TraceAI 关注的是更高维度的问题：

```text
❌ Claude Code 调用了 Bash 100 次
❌ Cursor 调用了 ReadFile 200 次

✅ GitHub MCP:
     create_issue      523次  ★★★★★
     list_issues      1280次  ★★★★★
     search_code       892次  ★★★★☆
     update_issue       17次  ★☆☆☆☆
     delete_branch       0次  —— 零调用

✅ Figma MCP:
     get_design        820次  ★★★★★
     export_asset       11次  ★☆☆☆☆
     create_frame        3次  ★☆☆☆☆
```

**Tool → Function 级别的分析才有产品决策价值。**

---

## 装了 12 个 MCP 插件，AI 真的在用几个？

这是每个用 AI 写代码的开发者迟早会面对的场景。你花了一下午配置 MCP：

```text
✅ GitHub MCP      —— 官方出品，肯定常用
✅ Figma MCP       —— 设计师说连接一下
✅ Linear MCP      —— 团队在 Linear 管任务
✅ Postgres MCP    —— 直接查数据库方便
✅ Slack MCP       —— 也许 AI 能帮我发消息
✅ Filesystem MCP  —— 读写文件总得用吧
✅ Jira MCP        —— 公司用 Jira
✅ Notion MCP      —— 文档都在 Notion
```

然后你继续写代码，AI 继续辅助你。

**一周后，你打开 TraceAI：**

```text
你的 MCP 使用报告
────────────────────────────────────────
Filesystem MCP    ████████████████████  78%  ← 每时每刻都在读写文件
GitHub MCP        ████████              15%  ← 查代码、提 PR，确实在用
Postgres MCP      ██                     4%  ← 偶尔查一下
Linear MCP        ▏                      2%  ← 几乎不用
Slack MCP         ▏                      1%  ← AI 从不主动发消息
────────────────────────────────────────
Figma MCP         (无调用)                     ← 设计师白装了
Jira MCP          (无调用)                     ← 是不是配置有问题？
Notion MCP        (无调用)                     ← AI 不知道 Notion 里有什么
```

**三个问题立刻浮现：**

1. **Figma MCP 零调用** — 是 AI 不知道设计师给了设计稿？还是 tool description 写得不好 AI 理解不了？
2. **Jira MCP 零调用** — 是我没在 Prompt 里提到要用 Jira？还是 Jira MCP 的 API 设计对 AI 不友好？
3. **Slack MCP 几乎零调用** — AI 确实不会主动发消息，这种"主动通知"类的工具，AI 天然不倾向调用。

**更深的发现：**

```text
Filesystem MCP 详细
────────────────────────────────────────
read_file          ████████████████  62%   读取远多于写入
write_to_file      ██████            18%
search_files       ████              12%
list_directory     ██                 6%
delete_files       ▏                  2%   ← AI 几乎不删除文件
move_files         (无调用)                 ← AI 不会帮你整理目录
```

你立刻就能判断：**Filesystem MCP 值了**，但那些零调用的插件——要么改进 Prompt 让 AI 知道它们的存在，要么卸载掉，省得它们白白占着 context window。

---

## 从"我装了什么"到"AI 用了什么"

这个转变才是 TraceAI 真正想做的事情。它不是让你统计自己调用了多少次工具，而是让你**看见 AI 的选择**。

当你装了 12 个 MCP Server，每个 Server 又暴露了十几个 tool，你的 AI 上下文里塞满了工具描述。但 AI 实际会主动调用的，可能只有其中的 20%。

剩下的 80%：
- 有些是"你希望 AI 用，但 AI 不知道什么时候该用"
- 有些是"工具描述写得像给人看的，AI 根本理解不了"
- 有些是"这个功能就不适合 AI 调用"

TraceAI 让这些沉默的工具**显形**。

---

## 谁需要这个？

| 角色 | 想解决的问题 |
|------|-------------|
| **用 AI 写代码的开发者** | 我装了 12 个 MCP 插件，AI 到底用了哪几个？哪些该卸载？ |
| **MCP Server 开发者** | 我的 20 个 tool 里，AI 实际用了哪几个？哪些该砍掉？ |
| **Agent 平台团队** | 不同 Agent（Claude Code / Cursor / LangChain）的行为模式有什么差异？ |
| **插件开发者** | 用户装了我的插件，但 AI 真的在调用吗？ |
| **API 设计者** | 我的 API 对 AI 友好吗？哪个 endpoint 错误率最高？ |
| **产品经理** | 下个版本优先优化哪个功能？数据说了算。 |

---

## 核心功能

### 🔍 功能热力图

```text
GitHub MCP
────────────────────────────────
★★★★★  search_code     1,280 calls   98.7% 成功
★★★★★  get_file           820 calls   99.1% 成功
★★★★☆  create_pr          523 calls   91.2% 成功
★★☆☆☆  merge_pr            45 calls   62.3% 成功   ⚠️ 高失败率
★☆☆☆☆  delete_branch        0 calls    ——    零调用
```

一眼看出：常用功能、增长功能、零调用功能、高失败率功能。

### 📊 失败率归因

```text
Tool              Calls    Errors    Rate
─────────────────────────────────────────
create_issue      1,000        5     0.5%
search_issue      3,000        8     0.3%
update_issue        500      180    36.0%   🚨 每三次调用就失败一次！
```

高失败率自动标记，帮助开发者定位 API 设计问题、文档缺陷或参数复杂度过高。

### 🤖 Agent 行为画像

```text
Claude Code    → 偏爱 search_*，几乎不写文件
Cursor         → 大量 read_file + edit，读多写少
LangChain      → 调用链长，retry 次数偏高
```

不同 Agent 的行为差异一目了然。如果某个 Agent 从不调用你的核心功能，可能是 Prompt 没引导，或者工具描述不够清晰。

### 🧠 AI 行为模式发现

```text
Database MCP
────────────────────────────────
query_table     query_table     query_table     query_table
query_table     query_table     query_table     query_table

从未调用：list_schema  ← AI 不知道数据库结构
```

这种模式揭示的是工具设计问题，不是用户问题。新增一个 `describe_database()` 就能改变 AI 的行为路径。

### 📈 月度趋势报告

```text
本月报告 — 2026-06
────────────────────────────────
最常用功能    search_code        ↑ 12%
增长最快      create_pr          ↑ 34%
高失败率      merge_pr           62.3%   ⚠️ 建议优先修复
零调用        delete_branch      0      考虑废弃或改进 Prompt
```

---

## 架构

```text
┌──────────┐   ┌──────────┐   ┌──────────┐   ┌──────────┐
│ MCP      │   │ OpenAI   │   │ Claude   │   │ Cursor   │
│ Adapter  │   │ Adapter  │   │ Adapter  │   │ Adapter  │   ...
└────┬─────┘   └────┬─────┘   └────┬─────┘   └────┬─────┘
     │              │              │              │
     └──────────────┴──────────────┴──────────────┘
                          │
                    ┌─────▼─────┐
                    │  Event    │  统一事件模型
                    │  Bus      │  批量落盘 · 失败重试
                    └─────┬─────┘
                          │
                    ┌─────▼─────┐
                    │  SQLite   │  本地优先 · 零依赖
                    │  Storage  │
                    └─────┬─────┘
                          │
              ┌───────────┼───────────┐
              │           │           │
        ┌─────▼─────┐ ┌──▼──┐ ┌─────▼─────┐
        │  CLI      │ │ SDK │ │ Dashboard │
        │  top-tools│ │ Go  │ │  Web UI   │
        │  stats    │ │     │ │           │
        └───────────┘ └─────┘ └───────────┘
```

TraceAI **不直接依赖任何 Agent 框架**。所有平台通过 Adapter 接入，产出统一的事件模型。

### 统一事件模型 (v1)

```json
{
  "event_id": "evt_abc123",
  "schema_version": "v1",
  "trace_id": "trc_xyz789",
  "session_id": "ses_def456",
  "timestamp": "2026-06-20T10:30:00Z",
  "agent_name": "Claude Code",
  "agent_version": "2.0.0",
  "adapter_name": "mcp",
  "adapter_version": "0.1.0",
  "tool_type": "mcp",
  "tool_name": "github",
  "function_name": "search_code",
  "success": true,
  "duration_ms": 245,
  "input_size": 1024,
  "output_size": 8192,
  "retry_count": 0,
  "error_type": "",
  "error_message": "",
  "metadata": {}
}
```

---

## 快速开始

### 安装

```bash
go build -o bin/toollens ./cmd/toollens
```

也支持环境变量和配置文件：

- `TRACEAI_STORE=sqlite|memory`
- `TRACEAI_DB=trace.db`
- `TRACEAI_CONFIG=path/to/config.json`

配置文件使用 JSON，示例：

```json
{"store":"sqlite","db":"trace.db"}
```

### 运行 CLI 分析

```bash
# 查看最常用的 MCP 工具
./bin/toollens --store sqlite --db trace.db top-tools

# 查看最常用的函数
./bin/toollens --store sqlite --db trace.db top-functions --limit 20

# 查看哪个 Agent 调用最多
./bin/toollens --store sqlite --db trace.db top-agents

# 查看总体统计
./bin/toollens --store sqlite --db trace.db stats

# 输出基础报表
./bin/toollens --store sqlite --db trace.db report --limit 5 --catalog tools.txt --trend-days 7

# 启动 Dashboard
./bin/toollens --store sqlite --db trace.db dashboard --addr :8080

# 查看运行状态
./bin/toollens --store sqlite --db trace.db status

# 健康检查
./bin/toollens --store sqlite --db trace.db health

# 指标输出
./bin/toollens --store sqlite --db trace.db metrics

# 显式写入演示数据
./bin/toollens --store sqlite --db trace.db seed-demo

# 导出 top tools 为 CSV
./bin/toollens --store sqlite --db trace.db export top-tools --format csv --output top-tools.csv

# 导出统计快照为 JSON
./bin/toollens --store sqlite --db trace.db export stats --format json --output stats.json

# 导出月报快照
./bin/toollens --store sqlite --db trace.db export monthly-stats --format json --output monthly-stats.json
```

Windows 下把 `./bin/toollens` 换成 `.\bin\toollens.exe` 即可。

### 集成 SDK

```go
import "toollens/pkg/sdk"

store, _ := storage.New(storage.Config{Backend: "sqlite", Path: "trace.db"})
tsdk := sdk.New(store)
tsdk.Start(ctx)

// 记录一次 MCP 工具调用
tsdk.Publish(events.ToolEvent{
    ToolName:     "github",
    FunctionName: "search_code",
    Success:      true,
    DurationMS:   245,
    // ...
})

// 查询分析结果
top, _ := tsdk.TopTools(ctx, time.Time{}, 10)
```

### 文档入口

- [事件样例](docs/event-sample.json)
- [事件模型](docs/event-schema.md)
- [接入说明](docs/integration.md)
- [指标口径](docs/metrics.md)
- [存储结构](docs/storage-schema.md)
- [发布检查清单](task/release-checklist.md)

### 真实接入示例

- [MCP 示例](examples/mcp/main.go)
- [OpenAI 示例](examples/openai/main.go)

### 更多 Adapter

- Claude
- Cursor
- LangChain
- LangGraph
- A2A

---

## 项目状态

**核心链路已完成：**

- [x] 统一事件模型 (`ToolEvent`)
- [x] SQLite 存储 + 自动迁移
- [x] 事件总线（批量落盘、背压控制）
- [x] MCP Adapter
- [x] OpenAI Adapter
- [x] CLI 统计命令 (`top-tools`, `top-functions`, `top-agents`, `stats`)
- [x] 核心路径测试

**MVP 已完成：**

- [x] 去掉 demo 数据的默认自动注入
- [x] 补数据导出（CSV / JSON）
- [x] 增加端到端烟雾测试
- [x] 补齐安装与使用文档
- [x] 增加真实接入示例
- [x] 增加基础报表输出
- [x] 完成版本与发布检查

**Phase 2 已完成：**

- [x] `Bus.Publish()` 满队列不再阻塞调用方
- [x] `Bus.Close()` 可重复调用
- [x] `pkg/models` 改为独立公共 DTO
- [x] 增加日志与错误上下文
- [x] 增加配置层
- [x] 增加健康检查与状态输出
- [x] 增加 Metrics 预留
- [x] 增加发布检查清单
- [x] 更多 Adapter
- [x] 更完整的分析报表
- [x] Dashboard

---

## 为什么是现在？

三个趋势正在同时发生：

1. **MCP 生态爆发** — 越来越多开发者发布 MCP Server，但没人知道 AI 实际怎么用这些工具
2. **Agent Tool 标准化** — A2A、MCP 等协议让工具调用可观测成为可能
3. **AI-Native 产品分析空白** — 传统产品分析（Amplitude、Mixpanel）是给人设计的，TraceAI 是给 AI 调用路径设计的

这是一块还没有被充分占领的领域。

---

## 为什么叫 TraceAI

Trace 有三层含义：

- **追踪**每一个 Tool Call 的完整生命周期
- **描绘**AI 使用工具的真实行为画像
- **溯源**失败原因，改进工具设计

---

## 许可证

TBD
