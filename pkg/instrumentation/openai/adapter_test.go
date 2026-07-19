package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
)

func TestAdapterParsesChatToolCallsAndPreservesBodies(t *testing.T) {
	requestBody := `{"model":"gpt-4.1","stream":false}`
	request := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(requestBody))
	request = request.WithContext(provider.WithContext(request.Context(), provider.ContextFields{
		TraceID:   "trc_1",
		SessionID: "ses_1",
		AgentName: "support-agent",
	}))
	adapter := NewAdapter(Config{ToolNamespaces: map[string]string{
		"search_code":  "github",
		"create_issue": "github",
	}})

	requestContext, err := adapter.ObserveRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if requestContext.AdapterName != "openai-chat-completions" || requestContext.ModelName != "gpt-4.1" {
		t.Fatalf("unexpected request context: %+v", requestContext)
	}
	replayedRequest, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(replayedRequest) != requestBody {
		t.Fatalf("request body changed: %q", replayedRequest)
	}

	responseBody := `{"id":"chatcmpl_1","model":"gpt-4.1","choices":[{"message":{"tool_calls":[{"id":"call_search","type":"function","function":{"name":"search_code","arguments":"{\"query\":\"auth\"}"}},{"id":"call_issue","type":"function","function":{"name":"create_issue","arguments":"{\"title\":\"bug\"}"}}]}}]}`
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"req_openai"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}
	decisions, err := adapter.ObserveResponse(requestContext, response)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected two decisions, got %+v", decisions)
	}
	if decisions[0].ToolCallID != "call_search" || decisions[0].ToolName != "github" {
		t.Fatalf("unexpected first decision: %+v", decisions[0])
	}
	if decisions[1].ToolCallID != "call_issue" || decisions[1].RequestID != "req_openai" {
		t.Fatalf("unexpected second decision: %+v", decisions[1])
	}
	if decisions[0].Metadata[provider.NamespaceInferredKey] != nil {
		t.Fatalf("configured namespace must not be inferred: %+v", decisions[0].Metadata)
	}
	replayedResponse, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(replayedResponse) != responseBody {
		t.Fatalf("response body changed: %q", replayedResponse)
	}
}

func TestAdapterParsesResponsesFunctionCall(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(`{"model":"gpt-4.1-mini"}`))
	adapter := NewAdapter()
	requestContext, err := adapter.ObserveRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{
  "id":"resp_1",
  "model":"gpt-4.1-mini",
  "output":[{"type":"function_call","call_id":"call_weather","name":"weather","arguments":"{\"city\":\"Shanghai\"}"}]
}`)),
	}
	decisions, err := adapter.ObserveResponse(requestContext, response)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 1 {
		t.Fatalf("expected one decision, got %+v", decisions)
	}
	decision := decisions[0]
	if decision.AdapterName != "openai-responses" || decision.ToolCallID != "call_weather" || decision.FunctionName != "weather" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if decision.Metadata[provider.NamespaceInferredKey] != true {
		t.Fatalf("expected inferred namespace metadata: %+v", decision.Metadata)
	}
}
