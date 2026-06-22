package models

import "time"

const SchemaVersion = "v1"

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

type DailyStat struct {
	StatDay         string
	Calls           int64
	Success         int64
	TotalDurationMS int64
	InputSize       int64
	OutputSize      int64
}

type MonthlyStat struct {
	StatMonth       string
	Calls           int64
	Success         int64
	TotalDurationMS int64
	InputSize       int64
	OutputSize      int64
}

type WeeklyStat struct {
	StatWeek        string
	Calls           int64
	Success         int64
	TotalDurationMS int64
	InputSize       int64
	OutputSize      int64
}

type ErrorBreakdown struct {
	ErrorType string
	ErrorCode string
	Category  string
	Calls     int64
	Failures  int64
}

func (e ToolEvent) Clone() ToolEvent {
	clone := e
	if e.Metadata != nil {
		clone.Metadata = make(map[string]interface{}, len(e.Metadata))
		for k, v := range e.Metadata {
			clone.Metadata[k] = v
		}
	}
	return clone
}

func (e ToolEvent) Normalize() ToolEvent {
	clone := e.Clone()
	if clone.SchemaVersion == "" {
		clone.SchemaVersion = SchemaVersion
	}
	if clone.Metadata == nil {
		clone.Metadata = map[string]interface{}{}
	}
	return clone
}
