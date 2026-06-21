package adapters

import (
	"errors"
	"testing"
	"time"
)

func TestAdapterEmitCall(t *testing.T) {
	cases := []struct {
		name    string
		adapter *baseAdapter
	}{
		{name: "claude", adapter: NewClaudeAdapter("1.2.3").baseAdapter},
		{name: "cursor", adapter: NewCursorAdapter("1.2.3").baseAdapter},
		{name: "langchain", adapter: NewLangChainAdapter("1.2.3").baseAdapter},
		{name: "langgraph", adapter: NewLangGraphAdapter("1.2.3").baseAdapter},
		{name: "a2a", adapter: NewA2AAdapter("1.2.3").baseAdapter},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.adapter.Name() != tc.name {
				t.Fatalf("unexpected name: %s", tc.adapter.Name())
			}
			if err := tc.adapter.Start(); err != nil {
				t.Fatal(err)
			}
			tc.adapter.emit("agent", "tool", "function", false, 50, 10, 20, errors.New("boom"))
			select {
			case event := <-tc.adapter.Events():
				if event.AdapterName != tc.name {
					t.Fatalf("unexpected adapter name: %+v", event)
				}
				if event.ToolType != tc.name {
					t.Fatalf("unexpected tool type: %+v", event)
				}
				if event.ErrorMessage != "boom" {
					t.Fatalf("unexpected error message: %+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("expected event")
			}
			if err := tc.adapter.Stop(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
