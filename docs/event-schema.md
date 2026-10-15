# TraceAI 事件模型

## 版本策略

- `schema_version` 采用 `vMAJOR.MINOR` 形式。
- 增加可选字段时提升 MINOR。
- 移除字段或改变语义时提升 MAJOR。
- 新版本读取旧事件时，缺失字段按默认值处理。

## 字段规则

- `event_id`、`trace_id`、`session_id` 必填。
- `request_id` 标识一次 provider 或 JSON-RPC 请求，`call_id` 标识其中的工具调用；执行器事件用 `parent_event_id` 回链 provider 决策。
- `attempt` 从 1 开始表示同一逻辑调用的第几次尝试，`retry_count = attempt - 1`。
- `timestamp` 使用 UTC。
- `metadata` 始终按对象处理，空值时使用空对象。
- `error_type` / `error_code` / `error_message` 仅在失败时填写。

错误类型统一分为 `transport_error`、`protocol_error`、`parameter_error`、`tool_execution_error`、`provider_error` 和 `timeout_error`，具体 code 保留协议或 provider 原值。

## 示例

参考 `docs/event-sample.json`。
