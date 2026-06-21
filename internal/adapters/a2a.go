package adapters

type A2AAdapter struct {
	*baseAdapter
}

func NewA2AAdapter(version string) *A2AAdapter {
	return &A2AAdapter{baseAdapter: newBaseAdapter("a2a", "a2a", version)}
}

func (a *A2AAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	a.emit(agentName, toolName, functionName, success, durationMS, inputSize, outputSize, err)
}
