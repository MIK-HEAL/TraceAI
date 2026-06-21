package adapters

import "github.com/MIK-HEAL/TraceAI/internal/events"

type Adapter interface {
	Name() string
	Start() error
	Stop() error
	Events() <-chan events.ToolEvent
}

