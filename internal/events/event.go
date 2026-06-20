package events

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const SchemaVersion = "v1"

type ToolEvent struct {
	EventID        string                 `json:"event_id"`
	SchemaVersion   string                 `json:"schema_version"`
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

func NewToolEvent() ToolEvent {
	now := time.Now().UTC()
	return ToolEvent{
		EventID:       newID("evt"),
		SchemaVersion:  SchemaVersion,
		TraceID:       newID("trc"),
		SessionID:     newID("ses"),
		Timestamp:     now,
		Metadata:      map[string]interface{}{},
	}
}

func (e ToolEvent) Validate() error {
	switch {
	case e.EventID == "":
		return errors.New("event_id is required")
	case e.SchemaVersion == "":
		return errors.New("schema_version is required")
	case e.Timestamp.IsZero():
		return errors.New("timestamp is required")
	case e.AdapterName == "":
		return errors.New("adapter_name is required")
	case e.ToolType == "":
		return errors.New("tool_type is required")
	case e.ToolName == "":
		return errors.New("tool_name is required")
	case e.FunctionName == "":
		return errors.New("function_name is required")
	}
	return nil
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

func (e ToolEvent) SizeBytes() (int64, error) {
	data, err := json.Marshal(e)
	if err != nil {
		return 0, err
	}
	return int64(len(data)), nil
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

func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf[:]))
}
