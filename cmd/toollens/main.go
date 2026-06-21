package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"toollens/internal/analytics"
	"toollens/internal/events"
	"toollens/internal/storage"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(argv []string, out io.Writer) error {
	fs := flag.NewFlagSet("toollens", flag.ExitOnError)
	backend := fs.String("store", "sqlite", "storage backend: sqlite or memory")
	dbPath := fs.String("db", "toollens.db", "sqlite database path")
	_ = fs.Parse(argv)

	args := fs.Args()
	if len(args) < 1 {
		printUsage(out)
		return nil
	}

	store, err := storage.New(storage.Config{Backend: *backend, Path: *dbPath})
	if err != nil {
		return err
	}
	if err := store.Init(context.Background()); err != nil {
		return err
	}
	defer store.Close()

	engine := analytics.NewEngine(store)

	switch args[0] {
	case "top-tools":
		return runTopTools(engine, args[1:], out)
	case "top-functions":
		return runTopFunctions(engine, args[1:], out)
	case "top-agents":
		return runTopAgents(engine, args[1:], out)
	case "stats":
		return runStats(engine, args[1:], out)
	case "seed-demo":
		if err := seedDemoData(store); err != nil {
			return err
		}
		_, err := fmt.Fprintln(out, "demo data inserted")
		return err
	default:
		printUsage(out)
		return nil
	}
}

func seedDemoData(store storage.Storage) error {
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
		if err := store.InsertEvent(context.Background(), event); err != nil {
			return err
		}
	}
	return nil
}

func runTopTools(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("top-tools", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args)
	rows, err := engine.TopTools(context.Background(), time.Time{}, *limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		_, err := fmt.Fprintf(out, "%s\tcalls=%d\tsuccess=%d\n", row.ToolName, row.Calls, row.Success)
		if err != nil {
			return err
		}
	}
	return nil
}

func runTopFunctions(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("top-functions", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args)
	rows, err := engine.TopFunctions(context.Background(), time.Time{}, *limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		_, err := fmt.Fprintf(out, "%s\tcalls=%d\tsuccess=%d\n", row.FunctionName, row.Calls, row.Success)
		if err != nil {
			return err
		}
	}
	return nil
}

func runTopAgents(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("top-agents", flag.ExitOnError)
	limit := fs.Int("limit", 10, "result limit")
	_ = fs.Parse(args)
	rows, err := engine.TopAgents(context.Background(), time.Time{}, *limit)
	if err != nil {
		return err
	}
	for _, row := range rows {
		_, err := fmt.Fprintf(out, "%s\tcalls=%d\tsuccess=%d\n", row.AgentName, row.Calls, row.Success)
		if err != nil {
			return err
		}
	}
	return nil
}

func runStats(engine *analytics.Engine, args []string, out io.Writer) error {
	_ = args
	stats, err := engine.Stats(context.Background(), time.Time{})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "calls=%d\nsuccess_rate=%.2f\navg_latency_ms=%.2f\ninput_size=%d\noutput_size=%d\n",
		stats.Calls, stats.SuccessRate, stats.AvgLatency, stats.InputSize, stats.OutputSize)
	return err
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`
ToolLens

Usage:
  toollens [--store sqlite|memory] [--db path] top-tools
  toollens [--store sqlite|memory] [--db path] top-functions
  toollens [--store sqlite|memory] [--db path] top-agents
  toollens [--store sqlite|memory] [--db path] stats
  toollens [--store sqlite|memory] [--db path] seed-demo
`))
}
