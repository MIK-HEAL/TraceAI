package traceai

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/MIK-HEAL/TraceAI/pkg/models"
)

type recordContextKey struct{}
type clientContextKey struct{}
type callInfoContextKey struct{}

type RecordState struct {
	Client *Client
	Info   CallInfo
	Start  models.ToolEvent
}

func WithClient(ctx context.Context, client *Client) context.Context {
	return context.WithValue(ctx, clientContextKey{}, client)
}

func ClientFromContext(ctx context.Context) (*Client, bool) {
	client, ok := ctx.Value(clientContextKey{}).(*Client)
	return client, ok
}

func WithCallInfo(ctx context.Context, info CallInfo) context.Context {
	return context.WithValue(ctx, callInfoContextKey{}, info)
}

func CallInfoFromContext(ctx context.Context) (CallInfo, bool) {
	info, ok := ctx.Value(callInfoContextKey{}).(CallInfo)
	return info, ok
}

func WithRecordState(ctx context.Context, state RecordState) context.Context {
	return context.WithValue(ctx, recordContextKey{}, state)
}

func RecordStateFromContext(ctx context.Context) (RecordState, bool) {
	state, ok := ctx.Value(recordContextKey{}).(RecordState)
	return state, ok
}

func RecordStart(ctx context.Context, client *Client, info CallInfo) context.Context {
	info = info.withDefaults()
	state := RecordState{
		Client: client,
		Info:   info,
		Start:  info.Start(),
	}
	ctx = WithClient(ctx, client)
	ctx = WithCallInfo(ctx, info)
	return WithRecordState(ctx, state)
}

func RecordFinish(ctx context.Context, success bool, inputSize, outputSize int64, err error) error {
	state, ok := RecordStateFromContext(ctx)
	if !ok || state.Client == nil {
		return nil
	}
	event := state.Info.Finish(state.Start, success, inputSize, outputSize, err)
	return state.Client.Publish(event)
}

func (i CallInfo) withDefaults() CallInfo {
	if i.AdapterName == "" {
		i.AdapterName = "traceai"
	}
	if i.ToolType == "" {
		i.ToolType = i.AdapterName
	}
	return i
}

type HTTPStatusError struct {
	Status  int
	Message string
}

func (e HTTPStatusError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("http status %d", e.Status)
}

func (e HTTPStatusError) StatusCode() int {
	return e.Status
}

// ErrorDetails lets instrumentation classify telemetry without changing the
// application error returned to the caller.
type ErrorDetails interface {
	TraceAIError() (errorType, errorCode string)
}

const (
	ErrorTransport     = "transport_error"
	ErrorProtocol      = "protocol_error"
	ErrorParameter     = "parameter_error"
	ErrorToolExecution = "tool_execution_error"
	ErrorProvider      = "provider_error"
	ErrorTimeout       = "timeout_error"
)

// ClassifyError exposes the stable error taxonomy used by all SDK adapters.
func ClassifyError(err error) (errorType, errorCode, message string) { return classifyError(err) }

func classifyError(err error) (string, string, string) {
	if err == nil {
		return "", "", ""
	}
	message := err.Error()
	var detailed ErrorDetails
	if errors.As(err, &detailed) {
		errorType, errorCode := detailed.TraceAIError()
		if errorType != "" || errorCode != "" {
			return errorType, errorCode, message
		}
	}
	switch {
	case errors.Is(err, context.Canceled):
		return ErrorTransport, "context_canceled", message
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorTimeout, "deadline_exceeded", message
	}
	var statusErr interface{ StatusCode() int }
	if errors.As(err, &statusErr) {
		if code := statusErr.StatusCode(); code > 0 {
			return ErrorTransport, fmt.Sprintf("http_%d", code), message
		}
	}
	var timeoutErr interface{ Timeout() bool }
	if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
		return ErrorTimeout, "timeout", message
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrorTimeout, "timeout", message
	}
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "invalid param"), strings.Contains(lower, "validation"), strings.Contains(lower, "schema"):
		return ErrorParameter, "invalid_parameters", message
	case strings.Contains(lower, "jsonrpc"), strings.Contains(lower, "protocol"):
		return ErrorProtocol, "protocol_error", message
	case strings.Contains(lower, "provider"), strings.Contains(lower, "rate limit"):
		return ErrorProvider, "provider_error", message
	default:
		return ErrorToolExecution, "execution_error", message
	}
}
