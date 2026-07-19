// Package provider contains provider-neutral function-calling context.
package provider

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const (
	ProviderNameKey      = "traceai.provider.name"
	ModelNameKey         = "traceai.model.name"
	APIFamilyKey         = "traceai.api.family"
	RequestIDKey         = "traceai.request.id"
	ToolCallIDKey        = "traceai.tool.call_id"
	ParentEventIDKey     = "traceai.tool.parent_event_id"
	NamespaceInferredKey = "traceai.tool.namespace_inferred"
	CaptureModeKey       = "traceai.capture.mode"
	CaptureStreamingKey  = "traceai.capture.streaming"
	CaptureModeProvider  = "provider"
	CaptureModeExecutor  = "executor"
)

type ContextFields struct {
	TraceID      string
	SessionID    string
	AgentName    string
	AgentVersion string
	Metadata     map[string]any
}

type RequestContext struct {
	ContextFields
	RequestID      string
	ProviderName   string
	ModelName      string
	APIFamily      string
	AdapterName    string
	Streaming      bool
	ToolNamespaces map[string]string
}

type ToolDecision struct {
	EventID       string
	TraceID       string
	SessionID     string
	RequestID     string
	ToolCallID    string
	ProviderName  string
	ModelName     string
	APIFamily     string
	AdapterName   string
	AgentName     string
	AgentVersion  string
	ToolName      string
	FunctionName  string
	ArgumentsSize int64
	Streaming     bool
	Metadata      map[string]any
}

type Adapter interface {
	Name() string
	Provider() string
	Match(*http.Request) bool
	ObserveRequest(*http.Request) (RequestContext, error)
	ObserveResponse(RequestContext, *http.Response) ([]ToolDecision, error)
}

type DecisionSink interface {
	Record(context.Context, []ToolDecision)
}

type contextKey struct{}

func WithContext(ctx context.Context, fields ContextFields) context.Context {
	return context.WithValue(ctx, contextKey{}, fields.clone())
}

func ContextFromContext(ctx context.Context) (ContextFields, bool) {
	fields, ok := ctx.Value(contextKey{}).(ContextFields)
	return fields.clone(), ok
}

func NewDecision(request RequestContext, toolCallID, functionName string, argumentsSize int64) ToolDecision {
	toolName, inferred := ResolveToolName(functionName, request.ToolNamespaces)
	metadata := cloneMetadata(request.Metadata)
	metadata[ProviderNameKey] = request.ProviderName
	metadata[ModelNameKey] = request.ModelName
	metadata[APIFamilyKey] = request.APIFamily
	metadata[RequestIDKey] = request.RequestID
	metadata[ToolCallIDKey] = toolCallID
	metadata[CaptureModeKey] = CaptureModeProvider
	metadata[CaptureStreamingKey] = request.Streaming
	if inferred {
		metadata[NamespaceInferredKey] = true
	}
	return ToolDecision{
		EventID:       newID("decision"),
		TraceID:       request.TraceID,
		SessionID:     request.SessionID,
		RequestID:     request.RequestID,
		ToolCallID:    toolCallID,
		ProviderName:  request.ProviderName,
		ModelName:     request.ModelName,
		APIFamily:     request.APIFamily,
		AdapterName:   request.AdapterName,
		AgentName:     request.AgentName,
		AgentVersion:  request.AgentVersion,
		ToolName:      toolName,
		FunctionName:  functionName,
		ArgumentsSize: argumentsSize,
		Streaming:     request.Streaming,
		Metadata:      metadata,
	}
}

func (d ToolDecision) ExecutionMetadata() map[string]any {
	metadata := cloneMetadata(d.Metadata)
	metadata[ProviderNameKey] = d.ProviderName
	metadata[ModelNameKey] = d.ModelName
	metadata[APIFamilyKey] = d.APIFamily
	metadata[RequestIDKey] = d.RequestID
	metadata[ToolCallIDKey] = d.ToolCallID
	metadata[ParentEventIDKey] = d.EventID
	metadata[CaptureModeKey] = CaptureModeExecutor
	metadata[CaptureStreamingKey] = d.Streaming
	return metadata
}

func ResolveToolName(functionName string, namespaces map[string]string) (string, bool) {
	if namespace := namespaces[functionName]; namespace != "" {
		return namespace, false
	}
	return functionName, true
}

type Registry struct {
	mu        sync.Mutex
	decisions map[string]ToolDecision
}

func NewRegistry() *Registry {
	return &Registry{decisions: make(map[string]ToolDecision)}
}

func (r *Registry) Record(_ context.Context, decisions []ToolDecision) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, decision := range decisions {
		if decision.ToolCallID == "" {
			continue
		}
		r.decisions[decision.ToolCallID] = cloneDecision(decision)
	}
}

func (r *Registry) Get(toolCallID string) (ToolDecision, bool) {
	if r == nil || toolCallID == "" {
		return ToolDecision{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	decision, ok := r.decisions[toolCallID]
	return cloneDecision(decision), ok
}

func (r *Registry) Take(toolCallID string) (ToolDecision, bool) {
	if r == nil || toolCallID == "" {
		return ToolDecision{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	decision, ok := r.decisions[toolCallID]
	if ok {
		delete(r.decisions, toolCallID)
	}
	return cloneDecision(decision), ok
}

func (r *Registry) Len() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.decisions)
}

func NewRequestID() string {
	return newID("req")
}

func cloneMetadata(input map[string]any) map[string]any {
	if len(input) == 0 {
		return map[string]any{}
	}
	output := make(map[string]any, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func (f ContextFields) clone() ContextFields {
	f.Metadata = cloneMetadata(f.Metadata)
	return f
}

func cloneDecision(decision ToolDecision) ToolDecision {
	decision.Metadata = cloneMetadata(decision.Metadata)
	return decision
}

func newID(prefix string) string {
	var buffer [8]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buffer[:]))
}
