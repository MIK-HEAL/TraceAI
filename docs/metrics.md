# TraceAI 指标口径

## calls

`calls` 表示事件条数，不去重。

## success_rate

`success_rate = 成功事件数 / 总事件数`。

## latency_ms

`latency_ms` 使用 `duration_ms`，单位为毫秒。

## input_size / output_size

- 单位统一为字节。
- 统计对象是最终写入事件里的 payload 大小。

## retry_count

`retry_count` 表示一次逻辑调用中的重试次数，不包含系统内部存储重试。
