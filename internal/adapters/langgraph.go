package adapters

type LangGraphAdapter struct {
	*baseAdapter
}

func NewLangGraphAdapter(version string) *LangGraphAdapter {
	return &LangGraphAdapter{baseAdapter: newBaseAdapter("langgraph", "langgraph", version)}
}

func (a *LangGraphAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	a.emit(agentName, toolName, functionName, success, durationMS, inputSize, outputSize, err)
}
