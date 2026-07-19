// Package tool records real function execution and links it to provider context.
package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
	"github.com/MIK-HEAL/TraceAI/pkg/models"
	"github.com/MIK-HEAL/TraceAI/pkg/traceai"
)

type Recorder interface {
	Publish(models.ToolEvent) error
}

type Handler[Input any, Output any] func(context.Context, Input) (Output, error)

// Wrap records the actual execution result. A telemetry write failure is
// intentionally ignored so it cannot change the application's tool result.
func Wrap[Input any, Output any](decision provider.ToolDecision, recorder Recorder, handler Handler[Input, Output]) Handler[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		var zero Output
		if handler == nil {
			return zero, fmt.Errorf("traceai tool handler is required")
		}
		if recorder == nil {
			return handler(ctx, input)
		}

		info := callInfo(decision)
		start := info.Start()
		output, err := handler(ctx, input)
		telemetryErr := classifyExecutionError(err)
		_ = recorder.Publish(info.Finish(start, err == nil, valueSize(input), valueSize(output), telemetryErr))
		return output, err
	}
}

func callInfo(decision provider.ToolDecision) traceai.CallInfo {
	toolName := decision.ToolName
	functionName := decision.FunctionName
	if toolName == "" {
		toolName = functionName
	}
	if functionName == "" {
		functionName = toolName
	}
	if functionName == "" {
		functionName = "function"
	}
	if toolName == "" {
		toolName = functionName
	}
	adapterName := decision.AdapterName
	if adapterName == "" {
		adapterName = "traceai-tool"
	}
	metadata := map[string]any{provider.CaptureModeKey: provider.CaptureModeExecutor}
	if decision.ProviderName != "" || decision.ToolCallID != "" || decision.EventID != "" {
		metadata = decision.ExecutionMetadata()
	}
	return traceai.CallInfo{
		TraceID:      decision.TraceID,
		SessionID:    decision.SessionID,
		AdapterName:  adapterName,
		AgentName:    decision.AgentName,
		AgentVersion: decision.AgentVersion,
		ToolType:     "function",
		ToolName:     toolName,
		FunctionName: functionName,
		Metadata:     metadata,
	}
}

func valueSize(value any) int64 {
	if value == nil {
		return 0
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0
	}
	return int64(len(encoded))
}

func classifyExecutionError(err error) error {
	if err == nil {
		return nil
	}
	var detailed traceai.ErrorDetails
	if errors.As(err, &detailed) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Error{Err: err, Type: "tool_timeout", Code: "tool_timeout"}
	}
	return Error{Err: err, Type: "tool_execution_error", Code: "tool_execution_error"}
}

type Error struct {
	Err  error
	Type string
	Code string
}

func (e Error) Error() string {
	if e.Err == nil {
		return e.Code
	}
	return e.Err.Error()
}

func (e Error) Unwrap() error {
	return e.Err
}

func (e Error) TraceAIError() (string, string) {
	return e.Type, e.Code
}

func InvalidArguments(err error) error {
	return Error{Err: err, Type: "invalid_arguments", Code: "invalid_arguments"}
}

func NotRegistered(err error) error {
	return Error{Err: err, Type: "tool_not_registered", Code: "tool_not_registered"}
}
