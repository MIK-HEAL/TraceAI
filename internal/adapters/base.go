package adapters

import (
	"sync"

	"github.com/MIK-HEAL/TraceAI/internal/events"
)

type baseAdapter struct {
	name      string
	toolType  string
	version   string
	events    chan events.ToolEvent
	closeOnce sync.Once
}

func newBaseAdapter(name, toolType, version string) *baseAdapter {
	if version == "" {
		version = "0.1.0"
	}
	return &baseAdapter{
		name:     name,
		toolType: toolType,
		version:  version,
		events:   make(chan events.ToolEvent, 32),
	}
}

func (a *baseAdapter) Name() string { return a.name }

func (a *baseAdapter) Start() error { return nil }

func (a *baseAdapter) Stop() error {
	a.closeOnce.Do(func() { close(a.events) })
	return nil
}

func (a *baseAdapter) Events() <-chan events.ToolEvent { return a.events }

func (a *baseAdapter) emit(agentName, toolName, functionName string, success bool, durationMS, inputSize, outputSize int64, err error) {
	capture := Capture{
		AdapterName:    a.Name(),
		AdapterVersion: a.version,
		AgentName:      agentName,
		ToolType:       a.toolType,
		ToolName:       toolName,
		FunctionName:   functionName,
	}
	event := capture.Start()
	event.Success = success
	event.DurationMS = durationMS
	event.InputSize = inputSize
	event.OutputSize = outputSize
	if err != nil {
		event.ErrorType = a.name + "_error"
		event.ErrorCode = a.name + "_error"
		event.ErrorMessage = err.Error()
	}
	a.events <- event
}
