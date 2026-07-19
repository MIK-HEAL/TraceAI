package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/anthropic"
	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/httptransport"
	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/provider"
	"github.com/MIK-HEAL/TraceAI/pkg/instrumentation/tool"
	"github.com/MIK-HEAL/TraceAI/pkg/traceai"
)

func main() {
	ctx := context.Background()
	client := traceai.New(traceai.NewMemoryStore())
	if err := client.Start(ctx); err != nil {
		panic(err)
	}
	defer func() { _ = client.Close(5 * time.Second) }()

	registry := provider.NewRegistry()
	modelClient := &http.Client{Transport: httptransport.NewTransport(
		http.DefaultTransport,
		registry,
		anthropic.NewAdapter(anthropic.Config{ToolNamespaces: map[string]string{
			"search_docs": "knowledge",
		}}),
	)}
	_ = modelClient // Pass this client to the Anthropic SDK or compatible HTTP client.

	registry.Record(ctx, []provider.ToolDecision{{
		EventID:      "decision_demo",
		TraceID:      "trc_demo",
		SessionID:    "ses_demo",
		RequestID:    "req_demo",
		ToolCallID:   "toolu_search_docs",
		ProviderName: "anthropic",
		ModelName:    "claude-sonnet-4-5",
		APIFamily:    "messages",
		AdapterName:  "anthropic-messages",
		AgentName:    "research-agent",
		ToolName:     "knowledge",
		FunctionName: "search_docs",
	}})
	decision, ok := registry.Take("toolu_search_docs")
	if !ok {
		panic("tool decision not captured")
	}

	searchDocs := tool.Wrap(decision, client, func(_ context.Context, input map[string]string) ([]string, error) {
		return []string{"matched: " + input["query"]}, nil
	})
	if _, err := searchDocs(ctx, map[string]string{"query": "retention policy"}); err != nil {
		panic(err)
	}
	stats, err := client.Stats(ctx, time.Time{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("anthropic execution stats: %+v\n", stats)
}
