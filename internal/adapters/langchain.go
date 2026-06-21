package adapters

type LangChainAdapter struct {
	*baseAdapter
}

func NewLangChainAdapter(version string) *LangChainAdapter {
	return &LangChainAdapter{baseAdapter: newBaseAdapter("langchain", "langchain", version)}
}

func (a *LangChainAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	a.emit(agentName, toolName, functionName, success, durationMS, inputSize, outputSize, err)
}
