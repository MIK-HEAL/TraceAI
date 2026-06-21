package adapters

type CursorAdapter struct {
	*baseAdapter
}

func NewCursorAdapter(version string) *CursorAdapter {
	return &CursorAdapter{baseAdapter: newBaseAdapter("cursor", "cursor", version)}
}

func (a *CursorAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	a.emit(agentName, toolName, functionName, success, durationMS, inputSize, outputSize, err)
}
