package adapters

import (
	"time"

	"toollens/internal/events"
)

type Capture struct {
	AdapterName    string
	AdapterVersion string
	AgentName      string
	AgentVersion   string
	ToolType       string
	ToolName       string
	FunctionName   string
	Metadata       map[string]interface{}
}

func (c Capture) Start() events.ToolEvent {
	e := events.NewToolEvent()
	e.AdapterName = c.AdapterName
	e.AdapterVersion = c.AdapterVersion
	e.AgentName = c.AgentName
	e.AgentVersion = c.AgentVersion
	e.ToolType = c.ToolType
	e.ToolName = c.ToolName
	e.FunctionName = c.FunctionName
	e.Metadata = cloneMetadata(c.Metadata)
	e.Timestamp = time.Now().UTC()
	return e
}

func (c Capture) Finish(start events.ToolEvent, success bool, inputSize, outputSize int64, err error) events.ToolEvent {
	event := start.Clone()
	event.Success = success
	event.InputSize = inputSize
	event.OutputSize = outputSize
	event.DurationMS = time.Since(start.Timestamp).Milliseconds()
	if err != nil {
		event.ErrorType = "adapter_error"
		event.ErrorMessage = err.Error()
	}
	return event
}

func cloneMetadata(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		out[k] = v
	}
	return out
}
