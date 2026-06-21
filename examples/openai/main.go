package main

import (
	"context"
	"fmt"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/adapters"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
	"github.com/MIK-HEAL/TraceAI/pkg/sdk"
)

func main() {
	ctx := context.Background()

	store := storage.NewMemoryStorage()
	client := sdk.New(store)
	if err := client.Start(ctx); err != nil {
		panic(err)
	}

	adapter := adapters.NewOpenAIAdapter("0.1.0")
	adapter.EmitCall("cursor", "chat.completions", "function_call", true, 210, 256, 512, nil)
	client.Publish(<-adapter.Events())

	waitForStoredEvents(ctx, store, 1)

	stats, err := client.Engine.Stats(ctx, time.Time{})
	if err != nil {
		panic(err)
	}
	fmt.Printf("stats: %+v\n", stats)

	client.Collector.Bus.Close()
}

func waitForStoredEvents(ctx context.Context, store storage.Storage, expected int) {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		rows, err := store.ListEvents(ctx, 10)
		if err == nil && len(rows) >= expected {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
