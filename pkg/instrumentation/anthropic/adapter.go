// Package anthropic observes Anthropic Messages tool-use responses, including SSE streams.
package anthropic

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
	return "anthropic-messages"
}

func (a *Adapter) Provider() string {
	return "anthropic"
}

func (a *Adapter) Match(request *http.Request) bool {
	return request != nil && request.Method == http.MethodPost && strings.TrimSuffix(request.URL.Path, "/") == "/v1/messages"
}

func (a *Adapter) ObserveRequest(request *http.Request) (provider.RequestContext, error) {
	if !a.Match(request) {
		return provider.RequestContext{}, fmt.Errorf("anthropic adapter does not match request")
	}
	body, complete, err := provider.ReadRequestBody(request, a.maxBodyBytes)
	if err != nil {
		return provider.RequestContext{}, err
	}
	if !complete {
		return provider.RequestContext{}, fmt.Errorf("anthropic request body exceeds capture limit")
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
	requestID := request.Header.Get("request-id")
	if requestID == "" {
		requestID = provider.NewRequestID()
	}
	return provider.RequestContext{
		ContextFields:  fields,
		RequestID:      requestID,
		ProviderName:   a.Provider(),
		ModelName:      payload.Model,
		APIFamily:      "messages",
		AdapterName:    a.Name(),
		Streaming:      payload.Stream,
		ToolNamespaces: cloneNamespaces(a.toolNamespaces),
	}, nil
}

func (a *Adapter) ObserveResponse(request provider.RequestContext, response *http.Response) ([]provider.ToolDecision, error) {
	if response == nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, nil
	}
	body, complete, err := provider.ReadResponseBody(response, a.maxBodyBytes)
	if err != nil {
		return nil, err
	}
	if !complete {
		return nil, nil
	}
	if request.Streaming || isStreaming(response) {
		return parseStreaming(request, response, body)
	}
	var payload struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string          `json:"type"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if requestID := response.Header.Get("request-id"); requestID != "" {
		request.RequestID = requestID
	} else if payload.ID != "" {
		request.RequestID = payload.ID
	}
	if payload.Model != "" {
		request.ModelName = payload.Model
	}
	decisions := make([]provider.ToolDecision, 0, len(payload.Content))
	for index, content := range payload.Content {
		if content.Type != "tool_use" || content.Name == "" {
			continue
		}
		toolCallID := content.ID
		if toolCallID == "" {
			toolCallID = request.RequestID + "-tool-" + strconv.Itoa(index)
		}
		decisions = append(decisions, provider.NewDecision(request, toolCallID, content.Name, int64(len(content.Input))))
	}
	return decisions, nil
}

func parseStreaming(request provider.RequestContext, response *http.Response, body []byte) ([]provider.ToolDecision, error) {
	frames, _ := provider.ParseSSEJSON(body)
	type call struct{ id, name, args string }
	calls := map[int]*call{}
	var responseID, model string
	for _, raw := range frames {
		var frame struct {
			Type    string `json:"type"`
			Message struct {
				ID    string `json:"id"`
				Model string `json:"model"`
			} `json:"message"`
			Index        int `json:"index"`
			ContentBlock struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type        string `json:"type"`
				PartialJSON string `json:"partial_json"`
			} `json:"delta"`
		}
		if json.Unmarshal(raw, &frame) != nil {
			continue
		}
		if frame.Message.ID != "" {
			responseID = frame.Message.ID
		}
		if frame.Message.Model != "" {
			model = frame.Message.Model
		}
		if frame.Type == "content_block_start" && frame.ContentBlock.Type == "tool_use" {
			calls[frame.Index] = &call{id: frame.ContentBlock.ID, name: frame.ContentBlock.Name}
		}
		if frame.Type == "content_block_delta" && frame.Delta.Type == "input_json_delta" {
			if calls[frame.Index] == nil {
				calls[frame.Index] = &call{}
			}
			calls[frame.Index].args += frame.Delta.PartialJSON
		}
	}
	requestID := response.Header.Get("request-id")
	if requestID != "" {
		request.RequestID = requestID
	} else if responseID != "" {
		request.RequestID = responseID
	}
	if model != "" {
		request.ModelName = model
	}
	decisions := make([]provider.ToolDecision, 0, len(calls))
	for i := 0; i < len(calls); i++ {
		c := calls[i]
		if c == nil || c.name == "" {
			continue
		}
		id := c.id
		if id == "" {
			id = request.RequestID + "-tool-" + strconv.Itoa(i)
		}
		decisions = append(decisions, provider.NewDecision(request, id, c.name, int64(len(c.args))))
	}
	return decisions, nil
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
