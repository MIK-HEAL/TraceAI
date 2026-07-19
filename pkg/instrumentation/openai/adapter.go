// Package openai observes non-streaming OpenAI function-calling responses.
package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
)

type Config struct {
	ToolNamespaces map[string]string
	MaxBodyBytes   int64
}

type Adapter struct {
	toolNamespaces map[string]string
	maxBodyBytes   int64
}

func NewAdapter(config ...Config) *Adapter {
	options := Config{MaxBodyBytes: provider.DefaultMaxBodyBytes}
	if len(config) > 0 {
		options = config[0]
		if options.MaxBodyBytes <= 0 {
			options.MaxBodyBytes = provider.DefaultMaxBodyBytes
		}
	}
	return &Adapter{
		toolNamespaces: cloneNamespaces(options.ToolNamespaces),
		maxBodyBytes:   options.MaxBodyBytes,
	}
}

func (a *Adapter) Name() string {
	return "openai"
}

func (a *Adapter) Provider() string {
	return "openai"
}

func (a *Adapter) Match(request *http.Request) bool {
	if request == nil || request.Method != http.MethodPost {
		return false
	}
	switch strings.TrimSuffix(request.URL.Path, "/") {
	case "/v1/responses", "/v1/chat/completions":
		return true
	default:
		return false
	}
}

func (a *Adapter) ObserveRequest(request *http.Request) (provider.RequestContext, error) {
	if !a.Match(request) {
		return provider.RequestContext{}, fmt.Errorf("openai adapter does not match request")
	}
	body, complete, err := provider.ReadRequestBody(request, a.maxBodyBytes)
	if err != nil {
		return provider.RequestContext{}, err
	}
	if !complete {
		return provider.RequestContext{}, fmt.Errorf("openai request body exceeds capture limit")
	}
	var payload struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &payload); err != nil {
			return provider.RequestContext{}, err
		}
	}
	fields, _ := provider.ContextFromContext(request.Context())
	apiFamily := "chat-completions"
	adapterName := "openai-chat-completions"
	if strings.TrimSuffix(request.URL.Path, "/") == "/v1/responses" {
		apiFamily = "responses"
		adapterName = "openai-responses"
	}
	requestID := request.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = provider.NewRequestID()
	}
	return provider.RequestContext{
		ContextFields:  fields,
		RequestID:      requestID,
		ProviderName:   a.Provider(),
		ModelName:      payload.Model,
		APIFamily:      apiFamily,
		AdapterName:    adapterName,
		Streaming:      payload.Stream,
		ToolNamespaces: cloneNamespaces(a.toolNamespaces),
	}, nil
}

func (a *Adapter) ObserveResponse(request provider.RequestContext, response *http.Response) ([]provider.ToolDecision, error) {
	if response == nil || request.Streaming || isStreaming(response) || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil
	}
	body, complete, err := provider.ReadResponseBody(response, a.maxBodyBytes)
	if err != nil {
		return nil, err
	}
	if !complete {
		return nil, nil
	}
	switch request.APIFamily {
	case "responses":
		return parseResponses(request, response, body)
	default:
		return parseChatCompletions(request, response, body)
	}
}

func parseResponses(request provider.RequestContext, response *http.Response, body []byte) ([]provider.ToolDecision, error) {
	var payload struct {
		ID     string `json:"id"`
		Model  string `json:"model"`
		Output []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	request = responseContext(request, response, payload.ID, payload.Model)
	decisions := make([]provider.ToolDecision, 0, len(payload.Output))
	for index, output := range payload.Output {
		if output.Type != "function_call" || output.Name == "" {
			continue
		}
		callID := output.CallID
		if callID == "" {
			callID = output.ID
		}
		if callID == "" {
			callID = fallbackCallID(request.RequestID, index)
		}
		decisions = append(decisions, provider.NewDecision(request, callID, output.Name, rawArgumentSize(output.Arguments)))
	}
	return decisions, nil
}

func parseChatCompletions(request provider.RequestContext, response *http.Response, body []byte) ([]provider.ToolDecision, error) {
	var payload struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				ToolCalls []struct {
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
				FunctionCall *struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function_call"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	request = responseContext(request, response, payload.ID, payload.Model)
	decisions := make([]provider.ToolDecision, 0)
	callIndex := 0
	for _, choice := range payload.Choices {
		for _, call := range choice.Message.ToolCalls {
			if call.Function.Name == "" {
				continue
			}
			callID := call.ID
			if callID == "" {
				callID = fallbackCallID(request.RequestID, callIndex)
			}
			decisions = append(decisions, provider.NewDecision(request, callID, call.Function.Name, int64(len(call.Function.Arguments))))
			callIndex++
		}
		if call := choice.Message.FunctionCall; call != nil && call.Name != "" {
			decisions = append(decisions, provider.NewDecision(request, fallbackCallID(request.RequestID, callIndex), call.Name, int64(len(call.Arguments))))
			callIndex++
		}
	}
	return decisions, nil
}

func responseContext(request provider.RequestContext, response *http.Response, responseID, model string) provider.RequestContext {
	if requestID := response.Header.Get("x-request-id"); requestID != "" {
		request.RequestID = requestID
	} else if responseID != "" {
		request.RequestID = responseID
	}
	if model != "" {
		request.ModelName = model
	}
	return request
}

func rawArgumentSize(raw json.RawMessage) int64 {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var arguments string
	if err := json.Unmarshal(raw, &arguments); err == nil {
		return int64(len(arguments))
	}
	return int64(len(raw))
}

func fallbackCallID(requestID string, index int) string {
	return requestID + "-call-" + strconv.Itoa(index)
}

func isStreaming(response *http.Response) bool {
	return strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream")
}

func cloneNamespaces(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
