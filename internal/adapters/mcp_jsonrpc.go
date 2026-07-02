// Package adapters provides platform adapters for TraceAI.
//
// MCP JSON-RPC 2.0 types and helpers.
// All MCP protocol-level knowledge (method names, param shapes, error codes)
// is confined to this file and mcp_proxy.go — no other package should import
// or depend on these types.
package adapters

import (
	"encoding/json"
	"fmt"
)

// ---------------------------------------------------------------------------
// JSON-RPC 2.0 base types
// ---------------------------------------------------------------------------

// JSONRPCMessage is a generic JSON-RPC 2.0 message.
// It can represent a request, response, or notification depending on which
// fields are populated:
//   - Request:  Method != "" && ID != nil
//   - Response: Method == "" && ID != nil
//   - Notification: Method != "" && ID == nil
type JSONRPCMessage struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method,omitempty"`
	Params  json.RawMessage  `json:"params,omitempty"`
	Result  json.RawMessage  `json:"result,omitempty"`
	Error   *JSONRPCError    `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// IDString returns the message id as a string for use as a map key.
func (m *JSONRPCMessage) IDString() string {
	if m.ID == nil {
		return ""
	}
	return string(*m.ID)
}

// IsRequest returns true when the message is a JSON-RPC request
// (has both method and id).
func (m *JSONRPCMessage) IsRequest() bool {
	return m.Method != "" && m.ID != nil
}

// IsResponse returns true when the message is a JSON-RPC response
// (has id but no method).
func (m *JSONRPCMessage) IsResponse() bool {
	return m.Method == "" && m.ID != nil
}

// IsNotification returns true when the message is a JSON-RPC notification
// (has method but no id).
func (m *JSONRPCMessage) IsNotification() bool {
	return m.Method != "" && m.ID == nil
}

// IsError returns true when the response carries an error.
func (m *JSONRPCMessage) IsError() bool {
	return m.Error != nil
}

// ---------------------------------------------------------------------------
// MCP protocol constants
// ---------------------------------------------------------------------------

// MCP method names as defined by the Model Context Protocol specification.
const (
	MCPMethodInitialize     = "initialize"
	MCPMethodInitialized    = "notifications/initialized"
	MCPMethodToolsList      = "tools/list"
	MCPMethodToolsCall      = "tools/call"
	MCPMethodToolsListChanged = "notifications/tools/list_changed"
	MCPMethodResourcesList  = "resources/list"
	MCPMethodResourcesRead  = "resources/read"
	MCPMethodPromptsList    = "prompts/list"
	MCPMethodPromptsGet     = "prompts/get"
)

// ---------------------------------------------------------------------------
// MCP parameter / result shapes (only what the proxy needs to inspect)
// ---------------------------------------------------------------------------

// MCPToolCallParams is the params shape for a tools/call request.
type MCPToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// MCPInitializeResult is the result shape for an initialize response.
type MCPInitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	ServerInfo      MCPServerInfo          `json:"serverInfo"`
	Capabilities    MCPServerCapabilities  `json:"capabilities"`
}

// MCPServerInfo carries the server identity returned during initialize.
type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// MCPServerCapabilities is the capabilities object in initialize result.
// We only model the fields the proxy needs; extra fields are preserved
// via json.RawMessage passthrough.
type MCPServerCapabilities struct {
	Tools *struct {
		ListChanged bool `json:"listChanged,omitempty"`
	} `json:"tools,omitempty"`
}

// MCPToolsListResult is the result shape for a tools/list response.
type MCPToolsListResult struct {
	Tools []MCPTool `json:"tools"`
}

// MCPTool describes a single tool returned by tools/list.
type MCPTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// ParseJSONRPCMessage unmarshals a single JSON-RPC message from raw bytes.
func ParseJSONRPCMessage(data []byte) (*JSONRPCMessage, error) {
	var msg JSONRPCMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("jsonrpc parse: %w", err)
	}
	if msg.JSONRPC != "2.0" {
		return nil, fmt.Errorf("jsonrpc: expected version 2.0, got %q", msg.JSONRPC)
	}
	return &msg, nil
}

// MarshalJSONRPCMessage serializes a JSON-RPC message to bytes.
func MarshalJSONRPCMessage(msg *JSONRPCMessage) ([]byte, error) {
	data, err := json.Marshal(msg)
	if err != nil {
		return nil, fmt.Errorf("jsonrpc marshal: %w", err)
	}
	return data, nil
}

// ParseToolCallParams extracts tools/call params from a message.
func ParseToolCallParams(msg *JSONRPCMessage) (*MCPToolCallParams, error) {
	var params MCPToolCallParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return nil, fmt.Errorf("parse tools/call params: %w", err)
	}
	return &params, nil
}

// ParseInitializeResult extracts server info from an initialize response.
func ParseInitializeResult(msg *JSONRPCMessage) (*MCPInitializeResult, error) {
	var result MCPInitializeResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, fmt.Errorf("parse initialize result: %w", err)
	}
	return &result, nil
}

// ParseToolsListResult extracts the tool list from a tools/list response.
func ParseToolsListResult(msg *JSONRPCMessage) (*MCPToolsListResult, error) {
	var result MCPToolsListResult
	if err := json.Unmarshal(msg.Result, &result); err != nil {
		return nil, fmt.Errorf("parse tools/list result: %w", err)
	}
	return &result, nil
}

// ClassifyJSONRPCError maps a JSON-RPC error code to a human-readable
// error type string suitable for ToolEvent.ErrorType.
func ClassifyJSONRPCError(code int) string {
	switch code {
	case -32700:
		return "jsonrpc_parse_error"
	case -32600:
		return "jsonrpc_invalid_request"
	case -32601:
		return "jsonrpc_method_not_found"
	case -32602:
		return "jsonrpc_invalid_params"
	case -32603:
		return "jsonrpc_internal_error"
	default:
		if code >= -32099 && code <= -32000 {
			return "jsonrpc_server_error"
		}
		return "jsonrpc_unknown_error"
	}
}
