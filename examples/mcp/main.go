package main

import (
	"context"
	"fmt"
	"time"

	"toollens/internal/adapters"
	"toollens/internal/storage"
	"toollens/pkg/sdk"
)

func main() {
	ctx := context.Background()

	store := storage.NewMemoryStorage()
	client := sdk.New(store)
	if err := client.Start(ctx); err != nil {
		panic(err)
	}

	adapter := adapters.NewMCPAdapter("0.1.0")
	adapter.EmitCall("claude-code", "github", "search_code", true, 245, 1024, 8192, nil)
	client.Publish(<-adapter.Events())

	waitForStoredEvents(ctx, store, 1)

	rows, err := client.TopTools(ctx, time.Time{}, 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("top tools: %+v\n", rows)

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
