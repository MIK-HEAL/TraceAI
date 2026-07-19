package httptransport

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/openai"
	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
)

func TestTransportCapturesDecisionsWithoutChangingBusinessBodies(t *testing.T) {
	requestBody := `{"model":"gpt-4.1"}`
	responseBody := `{"id":"chatcmpl_1","model":"gpt-4.1","choices":[{"message":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{}"}}]}}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != requestBody {
			t.Fatalf("server received changed request body: %q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-ID", "req_transport")
		_, _ = w.Write([]byte(responseBody))
	}))
	defer server.Close()

	registry := provider.NewRegistry()
	client := &http.Client{Transport: NewTransport(http.DefaultTransport, registry, openai.NewAdapter())}
	request, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != responseBody {
		t.Fatalf("caller received changed response body: %q", body)
	}
	decision, ok := registry.Take("call_1")
	if !ok || decision.RequestID != "req_transport" {
		t.Fatalf("expected captured decision, got %+v", decision)
	}
}

func TestTransportFailsOpenOnProviderParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer server.Close()

	registry := provider.NewRegistry()
	errors := 0
	client := &http.Client{Transport: &Transport{
		Base:     http.DefaultTransport,
		Sink:     registry,
		Adapters: []provider.Adapter{openai.NewAdapter()},
		OnError: func(error) {
			errors++
		},
	}}
	response, err := client.Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"gpt-4.1"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "not-json" || errors != 1 || registry.Len() != 0 {
		t.Fatalf("expected fail-open behavior, body=%q errors=%d decisions=%d", body, errors, registry.Len())
	}
}
