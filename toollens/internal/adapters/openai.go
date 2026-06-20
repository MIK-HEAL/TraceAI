package adapters

import (
	"toollens/internal/events"
)

type OpenAIAdapter struct {
	version string
	events  chan events.ToolEvent
}

func NewOpenAIAdapter(version string) *OpenAIAdapter {
	if version == "" {
		version = "0.1.0"
	}
	return &OpenAIAdapter{
		version: version,
		events:  make(chan events.ToolEvent, 32),
	}
}

func (a *OpenAIAdapter) Name() string { return "openai" }

func (a *OpenAIAdapter) Start() error { return nil }

func (a *OpenAIAdapter) Stop() error { close(a.events); return nil }

func (a *OpenAIAdapter) Events() <-chan events.ToolEvent { return a.events }

func (a *OpenAIAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	capture := Capture{
		AdapterName:    a.Name(),
		AdapterVersion: a.version,
		AgentName:      agentName,
		ToolType:       "openai",
		ToolName:       toolName,
		FunctionName:   functionName,
	}
	event := capture.Start()
	event.Success = success
	event.DurationMS = durationMS
	event.InputSize = inputSize
	event.OutputSize = outputSize
	if err != nil {
		event.ErrorType = "openai_error"
		event.ErrorMessage = err.Error()
	}
	a.events <- event
}
