package tool

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
	"github.com/MIK-HEAL/TraceAI/pkg/models"
	"github.com/MIK-HEAL/TraceAI/pkg/traceai"
)

func TestWrapRecordsRealExecutionWithProviderLink(t *testing.T) {
	ctx := context.Background()
	store := traceai.NewMemoryStore()
	client := &traceai.Client{Store: store}
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close(time.Second) })

	decision := provider.ToolDecision{
		EventID:      "decision_1",
		TraceID:      "trc_1",
		SessionID:    "ses_1",
		RequestID:    "req_1",
		ToolCallID:   "call_1",
		ProviderName: "openai",
		ModelName:    "gpt-4.1",
		APIFamily:    "chat-completions",
		AdapterName:  "openai-chat-completions",
		AgentName:    "support-agent",
		ToolName:     "github",
		FunctionName: "create_issue",
	}
	wrapped := Wrap(decision, client, func(_ context.Context, input map[string]string) (map[string]string, error) {
		return map[string]string{"issue": input["title"]}, nil
	})
	result, err := wrapped(ctx, map[string]string{"title": "bug"})
	if err != nil || result["issue"] != "bug" {
		t.Fatalf("unexpected result=%+v err=%v", result, err)
	}

	events, err := store.ListEvents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected one execution event, got %d", len(events))
	}
	event := events[0]
	if event.TraceID != "trc_1" || event.SessionID != "ses_1" || event.ToolType != "function" || !event.Success {
		t.Fatalf("unexpected execution event: %+v", event)
	}
	if event.Metadata[provider.ToolCallIDKey] != "call_1" || event.Metadata[provider.ParentEventIDKey] != "decision_1" {
		t.Fatalf("missing provider linkage: %+v", event.Metadata)
	}
	if event.InputSize == 0 || event.OutputSize == 0 {
		t.Fatalf("expected input/output sizes: %+v", event)
	}
}

func TestWrapRecordsExecutionFailureWithoutChangingBusinessError(t *testing.T) {
	recorder := &recordingRecorder{}
	businessErr := errors.New("upstream rejected request")
	wrapped := Wrap(provider.ToolDecision{ToolName: "github", FunctionName: "create_issue"}, recorder, func(context.Context, string) (string, error) {
		return "", businessErr
	})
	_, err := wrapped(context.Background(), "payload")
	if !errors.Is(err, businessErr) {
		t.Fatalf("expected original business error, got %v", err)
	}
	if len(recorder.events) != 1 {
		t.Fatalf("expected one event, got %d", len(recorder.events))
	}
	event := recorder.events[0]
	if event.Success || event.ErrorType != "tool_execution_error" || event.ErrorCode != "tool_execution_error" {
		t.Fatalf("unexpected failure event: %+v", event)
	}
}

func TestWrapIgnoresTelemetryWriteFailure(t *testing.T) {
	wrapped := Wrap(provider.ToolDecision{ToolName: "github", FunctionName: "search"}, failingRecorder{}, func(context.Context, string) (string, error) {
		return "ok", nil
	})
	result, err := wrapped(context.Background(), "query")
	if err != nil || result != "ok" {
		t.Fatalf("telemetry failure changed business result: result=%q err=%v", result, err)
	}
}

type recordingRecorder struct {
	events []models.ToolEvent
}

func (r *recordingRecorder) Publish(event models.ToolEvent) error {
	r.events = append(r.events, event)
	return nil
}

type failingRecorder struct{}

func (failingRecorder) Publish(models.ToolEvent) error {
	return errors.New("telemetry write failed")
}
