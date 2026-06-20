package adapters

import "toollens/internal/events"

type Adapter interface {
	Name() string
	Start() error
	Stop() error
	Events() <-chan events.ToolEvent
}

