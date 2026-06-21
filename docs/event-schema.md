# TraceAI 事件模型

## 版本策略

- `schema_version` 采用 `vMAJOR.MINOR` 形式。
- 增加可选字段时提升 MINOR。
- 移除字段或改变语义时提升 MAJOR。
- 新版本读取旧事件时，缺失字段按默认值处理。

## 字段规则

- `event_id`、`trace_id`、`session_id` 必填。
- `timestamp` 使用 UTC。
- `metadata` 始终按对象处理，空值时使用空对象。
- `error_type` / `error_code` / `error_message` 仅在失败时填写。

## 示例

参考 `docs/event-sample.json`。
