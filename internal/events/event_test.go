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

func TestNormalizeSetsDefaults(t *testing.T) {
	e := ToolEvent{}
	n := e.Normalize()
	if n.SchemaVersion != SchemaVersion {
		t.Fatalf("expected schema version %q, got %q", SchemaVersion, n.SchemaVersion)
	}
	if n.Metadata == nil {
		t.Fatal("expected metadata to be initialized")
	}
}
