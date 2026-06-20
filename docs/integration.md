# ToolLens 接入说明

## 事件模型

所有 adapter 都需要产出统一的 `ToolEvent`。

最少字段要求：

- `event_id`
- `schema_version`
- `trace_id`
- `session_id`
- `timestamp`
- `adapter_name`
- `tool_type`
- `tool_name`
- `function_name`
- `success`
- `duration_ms`
- `input_size`
- `output_size`

## 统计口径

- `calls` 统计事件条数。
- `success_rate` = 成功事件数 / 总事件数。
- `latency` 使用 `duration_ms` 的平均值。
- `input_size` / `output_size` 单位为字节。
- `retry_count` 表示同一次逻辑调用里的重试次数。

## Adapter 接入流程

1. 在调用开始时生成一个 `ToolEvent`。
2. 填充 adapter / agent / tool / function 等元数据。
3. 调用结束后回填 `success`、`duration_ms`、`input_size`、`output_size`。
4. 出错时补充 `error_type` 和 `error_message`。
5. 将事件写入事件总线或直接写入存储层。

## 示例

参考 `event-sample.json`。
