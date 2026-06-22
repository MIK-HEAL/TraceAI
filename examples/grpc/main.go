package main

import (
	"context"
	"fmt"
	"time"

	"github.com/MIK-HEAL/TraceAI/pkg/traceai"
)

func main() {
	ctx := context.Background()
	client := traceai.New(traceai.NewMemoryStore())
	if err := client.Start(ctx); err != nil {
		panic(err)
	}
	defer func() { _ = client.Close(5 * time.Second) }()

	if err := traceai.CaptureRPC(ctx, client, traceai.CallInfo{
		AdapterName:  "grpc",
		AgentName:    "claude-code",
		ToolType:     "grpc",
		ToolName:     "repo",
		FunctionName: "ListFiles",
	}, func(context.Context) (int64, int64, error) {
		return 4096, 8192, nil
	}); err != nil {
		panic(err)
	}

	rows, err := client.TopFunctions(ctx, time.Time{}, 10)
	if err != nil {
		panic(err)
	}
	fmt.Printf("grpc top functions: %+v\n", rows)
}
