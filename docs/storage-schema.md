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
