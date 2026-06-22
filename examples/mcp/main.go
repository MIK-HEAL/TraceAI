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

	if err := traceai.WrapMCP(client, traceai.CallInfo{
		AdapterName:  "mcp",
		AgentName:    "claude-code",
		ToolType:     "mcp",
		ToolName:     "github",
		FunctionName: "search_code",
	}, func(context.Context) error {
		rows, err := client.TopTools(ctx, time.Time{}, 10)
		if err != nil {
			return err
		}
		fmt.Printf("mcp top tools: %+v\n", rows)
		return nil
	})(ctx); err != nil {
		panic(err)
	}
}
