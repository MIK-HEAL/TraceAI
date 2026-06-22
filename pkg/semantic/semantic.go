package semantic

const (
	ToolName     = "traceai.tool.name"
	ToolType     = "traceai.tool.type"
	ToolSuccess  = "traceai.tool.success"
	ToolDuration = "traceai.tool.duration_ms"
	AgentName    = "traceai.agent.name"
	ErrorCode    = "traceai.error.code"
	ErrorType    = "traceai.error.type"
)

var All = []string{
	ToolName,
	ToolType,
	ToolSuccess,
	ToolDuration,
	AgentName,
	ErrorCode,
	ErrorType,
}
