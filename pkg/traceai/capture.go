package traceai

import (
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
)

type CallInfo struct {
	TraceID        string
	SessionID      string
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
	event := events.NewToolEvent()
	event.Timestamp = time.Now().UTC()
	if i.TraceID != "" {
		event.TraceID = i.TraceID
	}
	if i.SessionID != "" {
		event.SessionID = i.SessionID
	}
	event.AdapterName = i.AdapterName
	event.AdapterVersion = i.AdapterVersion
	event.AgentName = i.AgentName
	event.AgentVersion = i.AgentVersion
	event.ToolType = i.ToolType
	event.ToolName = i.ToolName
	event.FunctionName = i.FunctionName
	event.Metadata = cloneMetadata(i.Metadata)
	return models.ToolEvent(event)
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
