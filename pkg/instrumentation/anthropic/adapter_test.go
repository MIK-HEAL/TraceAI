package anthropic

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
)

func TestAdapterParsesMessagesToolUse(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "https://api.anthropic.com/v1/messages", strings.NewReader(`{"model":"claude-sonnet-4-5"}`))
	request = request.WithContext(provider.WithContext(request.Context(), provider.ContextFields{
		TraceID:   "trc_anthropic",
		SessionID: "ses_anthropic",
		AgentName: "research-agent",
	}))
	adapter := NewAdapter(Config{ToolNamespaces: map[string]string{"search_docs": "knowledge"}})
	requestContext, err := adapter.ObserveRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	responseBody := `{"id":"msg_1","model":"claude-sonnet-4-5","content":[{"type":"text","text":"I will look it up."},{"type":"tool_use","id":"toolu_search","name":"search_docs","input":{"query":"policy"}}]}`
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Request-Id": []string{"req_anthropic"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}
	decisions, err := adapter.ObserveResponse(requestContext, response)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected one decision, got %+v", decisions)
	}
	decision := decisions[0]
	if decision.ToolCallID != "toolu_search" || decision.ToolName != "knowledge" || decision.RequestID != "req_anthropic" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if decision.ArgumentsSize == 0 || decision.Metadata[provider.ProviderNameKey] != "anthropic" {
		t.Fatalf("missing safe provider metadata: %+v", decision)
	}
	replayedResponse, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(replayedResponse) != responseBody {
		t.Fatalf("response body changed: %q", replayedResponse)
	}
}
