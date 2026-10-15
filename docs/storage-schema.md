# TraceAI SQLite 表结构

## events

保存原始事件。

## sessions

保存会话级聚合。

## agents

保存 Agent 聚合统计。

## tools

保存工具聚合统计。

## daily_stats

保存按天聚合统计。

## 索引

- `events.timestamp`
- `events.tool_name`
- `events.agent_name`

## 数据生命周期与隐私

SQLite 可通过 `--capture-mode` 选择 `metadata-only`、`redacted`（默认）或
`full-payload`。redacted 会递归遮盖 prompt、arguments、input、output、token、
authorization 等敏感字段；metadata-only 直接丢弃这些字段。使用
`--retention-days N` 和 `cleanup` 删除过期事件，`--max-db-bytes` 可在清理后
检查数据库大小上限。清理会重建 sessions、agents、tools 和 daily_stats 聚合表。

MCP `tools/list` 目录保存在 `mcp_tool_catalog`，与执行事件分开，因此可以在
没有调用记录时分析零调用工具。
