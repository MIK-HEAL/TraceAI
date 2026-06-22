package traceai

import (
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
)

type CallInfo struct {
	AdapterName    string
	AdapterVersion string
	AgentName      string
	AgentVersion   string
	ToolType       string
	ToolName       string
	FunctionName   string
	Metadata       map[string]any
}

func (i CallInfo) Start() models.ToolEvent {
	event := models.ToolEvent{
		SchemaVersion:  "v1",
		Timestamp:      time.Now().UTC(),
		AdapterName:    i.AdapterName,
		AdapterVersion: i.AdapterVersion,
		AgentName:      i.AgentName,
		AgentVersion:   i.AgentVersion,
		ToolType:       i.ToolType,
		ToolName:       i.ToolName,
		FunctionName:   i.FunctionName,
		Metadata:       cloneMetadata(i.Metadata),
	}
	return event
}

func (i CallInfo) Finish(start models.ToolEvent, success bool, inputSize, outputSize int64, err error) models.ToolEvent {
	event := start.Clone()
	event.Success = success
	event.DurationMS = time.Since(start.Timestamp).Milliseconds()
	event.InputSize = inputSize
	event.OutputSize = outputSize
	if err != nil {
		event.ErrorType, event.ErrorCode, event.ErrorMessage = classifyError(err)
	}
	return event
}

func cloneMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
