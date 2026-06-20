package adapters

import (
	"toollens/internal/events"
)

type MCPAdapter struct {
	version string
	events  chan events.ToolEvent
}

func NewMCPAdapter(version string) *MCPAdapter {
	if version == "" {
		version = "0.1.0"
	}
	return &MCPAdapter{
		version: version,
		events:  make(chan events.ToolEvent, 32),
	}
}

func (a *MCPAdapter) Name() string { return "mcp" }

func (a *MCPAdapter) Start() error { return nil }

func (a *MCPAdapter) Stop() error { close(a.events); return nil }

func (a *MCPAdapter) Events() <-chan events.ToolEvent { return a.events }

func (a *MCPAdapter) EmitCall(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	capture := Capture{
		AdapterName:    a.Name(),
		AdapterVersion: a.version,
		AgentName:      agentName,
		ToolType:       "mcp",
		ToolName:       toolName,
		FunctionName:   functionName,
	}
	event := capture.Start()
	event.AgentName = agentName
	event.ToolName = toolName
	event.FunctionName = functionName
	event.Success = success
	event.DurationMS = durationMS
	event.InputSize = inputSize
	event.OutputSize = outputSize
	if err != nil {
		event.ErrorType = "mcp_error"
		event.ErrorMessage = err.Error()
	}
	a.events <- event
}
