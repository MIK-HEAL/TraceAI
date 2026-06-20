package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"toollens/internal/analytics"
	"toollens/internal/events"
	"toollens/internal/storage"
)

func main() {
	fs := flag.NewFlagSet("toollens", flag.ExitOnError)
	backend := fs.String("store", "sqlite", "storage backend: sqlite or memory")
	dbPath := fs.String("db", "toollens.db", "sqlite database path")
	_ = fs.Parse(os.Args[1:])

	args := fs.Args()
	if len(args) < 1 {
		printUsage()
		return
	}

	store, err := storage.New(storage.Config{Backend: *backend, Path: *dbPath})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	_ = store.Init(context.Background())
	defer store.Close()

	maybeSeedDemoData(store)

	engine := analytics.NewEngine(store)

	switch args[0] {
	case "top-tools":
		runTopTools(engine, args[1:])
	case "top-functions":
		runTopFunctions(engine, args[1:])
	case "top-agents":
		runTopAgents(engine, args[1:])
	case "stats":
		runStats(engine, args[1:])
	default:
		printUsage()
	}
}

func seedDemoData(store storage.Storage) {
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		event := events.NewToolEvent()
		event.Timestamp = now.Add(-time.Duration(i) * time.Minute)
		event.AgentName = "demo-agent"
		event.AgentVersion = "0.1.0"
		event.AdapterName = "mcp"
		event.AdapterVersion = "0.1.0"
		event.ToolType = "mcp"
		event.ToolName = []string{"search", "read", "search", "write", "search"}[i]
		event.FunctionName = []string{"tool_call", "tool_call", "tool_call", "tool_call", "tool_call"}[i]
		event.Success = i != 3
		event.DurationMS = int64(50 + i*20)
		event.InputSize = int64(100 + i*10)
		event.OutputSize = int64(80 + i*5)
		_ = store.InsertEvent(context.Background(), event)
	}
}

func maybeSeedDemoData(store storage.Storage) {
	events, err := store.ListEvents(context.Background(), 1)
	if err != nil || len(events) > 0 {
		return
	}
	seedDemoData(store)
}

func runTopTools(engine *analytics.Engine, args []string) {
	fs := flag.NewFlagSet("top-tools", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args)
	rows, _ := engine.TopTools(context.Background(), time.Time{}, *limit)
	for _, row := range rows {
		fmt.Printf("%s\tcalls=%d\tsuccess=%d\n", row.ToolName, row.Calls, row.Success)
	}
}

func runTopFunctions(engine *analytics.Engine, args []string) {
	fs := flag.NewFlagSet("top-functions", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args)
	rows, _ := engine.TopFunctions(context.Background(), time.Time{}, *limit)
	for _, row := range rows {
		fmt.Printf("%s\tcalls=%d\tsuccess=%d\n", row.FunctionName, row.Calls, row.Success)
	}
}

func runTopAgents(engine *analytics.Engine, args []string) {
	fs := flag.NewFlagSet("top-agents", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args)
	rows, _ := engine.TopAgents(context.Background(), time.Time{}, *limit)
	for _, row := range rows {
		fmt.Printf("%s\tcalls=%d\tsuccess=%d\n", row.AgentName, row.Calls, row.Success)
	}
}

func runStats(engine *analytics.Engine, args []string) {
	_ = args
	stats, _ := engine.Stats(context.Background(), time.Time{})
	fmt.Printf("calls=%d\nsuccess_rate=%.2f\navg_latency_ms=%.2f\ninput_size=%d\noutput_size=%d\n",
		stats.Calls, stats.SuccessRate, stats.AvgLatency, stats.InputSize, stats.OutputSize)
}

func printUsage() {
	fmt.Println(strings.TrimSpace(`
ToolLens

Usage:
  toollens [--store sqlite|memory] [--db path] top-tools
  toollens [--store sqlite|memory] [--db path] top-functions
  toollens [--store sqlite|memory] [--db path] top-agents
  toollens [--store sqlite|memory] [--db path] stats
`))
}
