# TraceAI Skill

## 这份手册做什么

帮助 AI 正确安装、验证、运行和排障 TraceAI CLI。优先解决三类任务：

- 安装和升级 TraceAI
- 验证 CLI、存储和指标是否正常
- 处理 release / build / config / database 的常见问题

## 这个项目怎么用

TraceAI 的使用方式很简单：先把 Agent 的工具调用记录下来，再用 CLI 去看这些调用到底发生了什么。

典型流程是：

1. 安装或构建 `traceai`
2. 让你的 Agent / MCP / SDK 写入事件
3. 用 `traceai health` 确认数据库可用
4. 用 `traceai metrics` 看整体运行状态
5. 用 `traceai report` 看工具热力图、失败率、Agent 使用情况
6. 用 `traceai export` 把结果导出去给别人看或继续分析

如果你只是试用，可以先跑 `seed-demo` 生成少量示例数据，再看报表。

## 使用效果是什么

用完 TraceAI 后，你通常会得到这几类结果：

- 哪些工具最常用
- 哪些功能几乎没人用
- 哪些 API 容易失败
- 哪些 Agent 更爱读文件、写文件或搜索
- 哪些错误更像参数设计问题，而不是代码故障

它的价值不是“看调用次数”，而是“看 AI 到底怎么选工具”。

## 什么时候用

在这些场景优先使用：

- 用户让你安装 TraceAI
- 用户让你确认 `traceai` 是否可用
- 用户让你检查 `health`、`metrics`、`report`、`export`
- 用户让你验证 release 或本地构建
- 用户遇到数据库路径、权限、配置文件、运行错误

不适合用在这些场景：

- 只想看产品路线或设计文档
- 只想讨论 TraceAI 的功能价值
- 不需要执行命令

## 先做判断

先判断用户是在做哪件事：

1. **安装**：走 `go install` 或本地 `go build`
2. **验证**：走 `version -> health -> metrics`
3. **分析**：走 `report -> export`
4. **排障**：先看配置、数据库、权限、日志

## 默认约定

- 对外统一使用 `traceai` 作为命令名
- 默认数据库名使用 `traceai.db`
- 默认存储后端优先使用 SQLite
- 除非用户明确要求，不要自动写入 demo 数据
- 本地 checkout 优先用本地 build，不优先联网安装

## 最小验证链路

如果只是要确认“能不能跑”，按这个顺序：

```bash
traceai version
traceai health
traceai metrics --format json
```

预期：

- `version` 能输出版本、commit、date
- `health` 返回 `ok`
- `metrics` 至少能输出一组指标

## 使用示例

### 示例 1: 本地快速试用

先构建并看版本：

```bash
go build -o bin/traceai ./cmd/traceai
./bin/traceai version
```

如果想看空库是否正常：

```bash
./bin/traceai health
./bin/traceai metrics --format json
```

如果想立刻看到报表：

```bash
./bin/traceai seed-demo
./bin/traceai report --limit 5
```

预期效果：

- 版本命令能确认程序可执行
- 健康检查能确认存储没问题
- 报表能看到工具热力图、失败率、Agent 使用情况

### 示例 2: 分析真实 MCP 使用情况

```bash
traceai report --limit 10
traceai export top-tools --format csv --output top-tools.csv
```

预期效果：

- 你能看出哪些 MCP tool 被频繁调用
- 你能看出哪些功能基本没人用
- 你能把结果导出给开发者继续分析

### 示例 3: 排查健康问题

```bash
traceai version
traceai health
traceai metrics --format json
```

预期效果：

- 如果 `version` 正常但 `health` 失败，大概率是存储或路径问题
- 如果 `metrics` 能出数据，说明运行链路基本正常
- 如果连 `version` 都不行，先看安装和构建是否成功

## 安装与构建

### 发布版安装

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@latest
```

### 指定版本安装

```bash
go install github.com/MIK-HEAL/TraceAI/cmd/traceai@v0.1.0-beta.1
```

### 本地构建

```bash
go build -o bin/traceai ./cmd/traceai
```

## 常用命令

```bash
traceai version
traceai health
traceai metrics --format json
traceai report --limit 5
traceai export top-tools --format csv --output top-tools.csv
```

## 命令结果怎么看

- `version`：确认二进制、版本注入是否正确
- `health`：确认存储可用、服务可用
- `metrics`：确认运行指标能导出
- `report`：确认分析输出可读
- `export`：确认结果能落盘给人或别的工具使用

## 你可以期待什么输出

典型的 `report` 会包含：

- Tool Heatmap
- Error Rate Ranking
- Behavior Profile
- Failure Reasons
- Agent Usage
- Trend

典型的 `export` 会得到：

- CSV 文件，适合表格查看
- JSON 文件，适合自动处理或二次分析

## 排障顺序

1. 先跑 `traceai version`
2. 再跑 `traceai health`
3. 再跑 `traceai metrics --format json`
4. 检查 `TRACEAI_STORE`、`TRACEAI_DB`、`TRACEAI_CONFIG`、`TRACEAI_LOG_LEVEL`、`TRACEAI_LOG_FORMAT`
5. 如果存储失败，检查 SQLite 路径和权限
6. 如果 release 验证失败，重新跑 `go test ./...` 和 `go build ./cmd/traceai`

## 常见反模式

- 不要在用户没有要求时默认 `seed-demo`
- 不要把 `health` 失败解释成“只是没数据”
- 不要把 `export` 的空结果误判成命令失败
- 不要先猜测问题，先用 `version` 和 `health` 缩小范围
