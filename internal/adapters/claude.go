package adapters

type ClaudeAdapter struct {
	*baseAdapter
}

func NewClaudeAdapter(version string) *ClaudeAdapter {
	return &ClaudeAdapter{baseAdapter: newBaseAdapter("claude", "claude", version)}
}

func (a *ClaudeAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	a.emit(agentName, toolName, functionName, success, durationMS, inputSize, outputSize, err)
}
