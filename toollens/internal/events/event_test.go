package events

import "testing"

func TestNewToolEventValidates(t *testing.T) {
	e := NewToolEvent()
	e.AdapterName = "mcp"
	e.ToolType = "mcp"
	e.ToolName = "search"
	e.FunctionName = "tool_call"
	if err := e.Validate(); err != nil {
		t.Fatalf("expected valid event, got error: %v", err)
	}
}
