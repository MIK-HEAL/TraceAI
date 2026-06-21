package models

import "time"

type ToolEvent struct {
	EventID        string                 `json:"event_id"`
	SchemaVersion  string                 `json:"schema_version"`
	TraceID        string                 `json:"trace_id"`
	SessionID      string                 `json:"session_id"`
	Timestamp      time.Time              `json:"timestamp"`
	AgentName      string                 `json:"agent_name"`
	AgentVersion   string                 `json:"agent_version"`
	AdapterName    string                 `json:"adapter_name"`
	AdapterVersion string                 `json:"adapter_version"`
	ToolType       string                 `json:"tool_type"`
	ToolName       string                 `json:"tool_name"`
	FunctionName   string                 `json:"function_name"`
	Success        bool                   `json:"success"`
	DurationMS     int64                  `json:"duration_ms"`
	InputSize      int64                  `json:"input_size"`
	OutputSize     int64                  `json:"output_size"`
	RetryCount     int64                  `json:"retry_count"`
	ErrorType      string                 `json:"error_type,omitempty"`
	ErrorCode      string                 `json:"error_code,omitempty"`
	ErrorMessage   string                 `json:"error_message,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

type ToolCount struct {
	ToolName string
	Calls    int64
	Success  int64
}

type FunctionCount struct {
	FunctionName string
	Calls        int64
	Success      int64
}

type AgentCount struct {
	AgentName string
	Calls     int64
	Success   int64
}

type ToolFailureRate struct {
	ToolName    string
	Calls       int64
	Failures    int64
	FailureRate float64
}

type Stats struct {
	Calls       int64
	SuccessRate float64
	AvgLatency  float64
	InputSize   int64
	OutputSize  int64
}
