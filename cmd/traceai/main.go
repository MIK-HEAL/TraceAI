package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/analytics"
	"github.com/MIK-HEAL/TraceAI/internal/config"
	"github.com/MIK-HEAL/TraceAI/internal/dashboard"
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/logging"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
	"github.com/MIK-HEAL/TraceAI/pkg/sdk"
	"github.com/MIK-HEAL/TraceAI/pkg/state"
)

var (
	version     = "dev"
	buildCommit = "unknown"
	buildDate   = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(argv []string, out io.Writer) error {
	fs := flag.NewFlagSet("traceai", flag.ExitOnError)
	cfg := config.Load(fs)
	_ = fs.Parse(argv)
	if err := config.ApplyFile(fs, cfg); err != nil {
		return wrapError("load config", err)
	}
	logging.Configure(cfg.LogLevel, cfg.LogFormat, os.Stderr)

	args := fs.Args()
	if len(args) < 1 {
		printUsage(out)
		return nil
	}
	slog.Default().With("component", "cli", "command", args[0]).Info("command start")
	defer slog.Default().With("component", "cli", "command", args[0]).Info("command finish")

	var store storage.Storage
	var engine *analytics.Engine
	if commandNeedsStorage(args[0]) {
		var err error
		store, err = storage.New(storage.Config{Backend: cfg.Store, Path: cfg.DB})
		if err != nil {
			return wrapError("create storage", err)
		}
		if err := store.Init(context.Background()); err != nil {
			return wrapError("init storage", err)
		}
		defer store.Close()
	}
	if store != nil {
		engine = analytics.NewEngine(store)
	}

	switch args[0] {
	case "version":
		return runVersion(out)
	case "top-tools":
		return runTopTools(engine, args[1:], out)
	case "top-functions":
		return runTopFunctions(engine, args[1:], out)
	case "top-agents":
		return runTopAgents(engine, args[1:], out)
	case "stats":
		return runStats(engine, args[1:], out)
	case "report":
		return runReport(engine, args[1:], out)
	case "dashboard":
		return runDashboard(store, args[1:], out)
	case "status":
		return runStatus(sdk.New(store), out)
	case "health":
		return runHealth(sdk.New(store), out)
	case "metrics":
		return runMetrics(sdk.New(store), args[1:], out)
	case "export":
		return runExport(engine, args[1:], out)
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

func commandNeedsStorage(command string) bool {
	switch command {
	case "top-tools", "top-functions", "top-agents", "stats", "report", "dashboard", "status", "health", "metrics", "export", "seed-demo":
		return true
	case "version":
		return false
	default:
		return false
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
		if _, err := fmt.Fprintf(out, "%s\tcalls=%d\tsuccess=%d\n", row.ToolName, row.Calls, row.Success); err != nil {
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
		if _, err := fmt.Fprintf(out, "%s\tcalls=%d\tsuccess=%d\n", row.FunctionName, row.Calls, row.Success); err != nil {
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
		if _, err := fmt.Fprintf(out, "%s\tcalls=%d\tsuccess=%d\n", row.AgentName, row.Calls, row.Success); err != nil {
			return err
		}
	}
	return nil
}

func runStats(engine *analytics.Engine, args []string, out io.Writer) error {
	_ = args
	stats, err := engine.Stats(context.Background(), time.Time{})
	if err != nil {
		return wrapError("query stats", err)
	}
	_, err = fmt.Fprintf(out, "calls=%d\nsuccess_rate=%.2f\navg_latency_ms=%.2f\ninput_size=%d\noutput_size=%d\n",
		stats.Calls, stats.SuccessRate, stats.AvgLatency, stats.InputSize, stats.OutputSize)
	return err
}

func runStatus(client *sdk.SDK, out io.Writer) error {
	status, err := client.Status(context.Background())
	if err != nil {
		_ = writeStatus(out, status)
		return wrapError("collect status", err)
	}
	return writeStatus(out, status)
}

func runHealth(client *sdk.SDK, out io.Writer) error {
	status, err := client.Status(context.Background())
	if err != nil {
		return wrapError("health check", err)
	}
	if !status.StorageOK || status.LastError != "" {
		if status.LastError != "" {
			return wrapError("health check", errors.New(status.LastError))
		}
		return errors.New("storage check failed")
	}
	_, err = fmt.Fprintln(out, "ok")
	return err
}

func runVersion(out io.Writer) error {
	_, err := fmt.Fprintf(out, "version=%s commit=%s date=%s\n", version, buildCommit, buildDate)
	return err
}

func runMetrics(client *sdk.SDK, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("metrics", flag.ExitOnError)
	format := fs.String("format", "text", "output format: text or json")
	_ = fs.Parse(args)

	metrics, err := client.Metrics(context.Background())
	if err != nil {
		return wrapError("collect metrics", err)
	}
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Name < metrics[j].Name
	})
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		return writeJSON(out, metrics)
	case "text":
		for _, metric := range metrics {
			if _, err := fmt.Fprintf(out, "%s=%v\n", metric.Name, metric.Value); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("unsupported metrics format %q", *format)
	}
}

func runReport(engine *analytics.Engine, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	limit := fs.Int("limit", 5, "result limit")
	catalog := fs.String("catalog", "", "expected tools catalog (file path or comma-separated list)")
	trendDays := fs.Int("trend-days", 7, "days to include in the trend section")
	_ = fs.Parse(args)

	report, err := makeReport(context.Background(), engine, *limit, *catalog, *trendDays)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, report)
	return err
}

func runDashboard(store storage.Storage, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("dashboard", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:8080", "listen address")
	token := fs.String("token", "", "dashboard bearer token")
	_ = fs.Parse(args)

	slog.Default().With("component", "cli", "command", "dashboard", "addr", *addr).Info("dashboard starting")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return dashboard.New(store).Serve(ctx, *addr, *token)
}

func runExport(engine *analytics.Engine, args []string, out io.Writer) error {
	if len(args) < 1 {
		return errors.New("export target required")
	}

	target := args[0]
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	format := fs.String("format", "csv", "export format: csv or json")
	outputPath := fs.String("output", "", "write output to file")
	_ = fs.Parse(args[1:])

	writer, closeFn, err := openOutput(out, *outputPath)
	if err != nil {
		return err
	}
	defer func() {
		_ = closeFn()
	}()

	switch target {
	case "top-tools":
		rows, err := engine.TopTools(context.Background(), time.Time{}, 0)
		if err != nil {
			return err
		}
		return writeTopToolsExport(writer, *format, rows)
	case "top-functions":
		rows, err := engine.TopFunctions(context.Background(), time.Time{}, 0)
		if err != nil {
			return err
		}
		return writeTopFunctionsExport(writer, *format, rows)
	case "top-agents":
		rows, err := engine.TopAgents(context.Background(), time.Time{}, 0)
		if err != nil {
			return err
		}
		return writeTopAgentsExport(writer, *format, rows)
	case "stats":
		stats, err := engine.Stats(context.Background(), time.Time{})
		if err != nil {
			return err
		}
		return writeStatsExport(writer, *format, stats)
	case "daily-stats":
		rows, err := engine.DailyStats(context.Background(), time.Time{})
		if err != nil {
			return err
		}
		return writeDailyStatsExport(writer, *format, rows)
	case "weekly-stats":
		rows, err := engine.WeeklyStats(context.Background(), time.Time{})
		if err != nil {
			return err
		}
		return writeWeeklyStatsExport(writer, *format, rows)
	case "monthly-stats":
		rows, err := engine.MonthlyStats(context.Background(), time.Time{})
		if err != nil {
			return err
		}
		return writeMonthlyStatsExport(writer, *format, rows)
	default:
		return fmt.Errorf("unknown export target %q", target)
	}
}

func writeTopToolsExport(w io.Writer, format string, rows []storage.ToolCount) error {
	switch format {
	case "json":
		payload := make([]exportToolRow, 0, len(rows))
		for _, row := range rows {
			payload = append(payload, exportToolRow{
				ToolName: row.ToolName,
				Calls:    row.Calls,
				Success:  row.Success,
			})
		}
		return writeJSON(w, payload)
	case "csv":
		records := make([][]string, 0, len(rows))
		for _, row := range rows {
			records = append(records, []string{
				row.ToolName,
				strconv.FormatInt(row.Calls, 10),
				strconv.FormatInt(row.Success, 10),
			})
		}
		return writeCSV(w, []string{"tool_name", "calls", "success"}, records)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeTopFunctionsExport(w io.Writer, format string, rows []storage.FunctionCount) error {
	switch format {
	case "json":
		payload := make([]exportFunctionRow, 0, len(rows))
		for _, row := range rows {
			payload = append(payload, exportFunctionRow{
				FunctionName: row.FunctionName,
				Calls:        row.Calls,
				Success:      row.Success,
			})
		}
		return writeJSON(w, payload)
	case "csv":
		records := make([][]string, 0, len(rows))
		for _, row := range rows {
			records = append(records, []string{
				row.FunctionName,
				strconv.FormatInt(row.Calls, 10),
				strconv.FormatInt(row.Success, 10),
			})
		}
		return writeCSV(w, []string{"function_name", "calls", "success"}, records)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeTopAgentsExport(w io.Writer, format string, rows []storage.AgentCount) error {
	switch format {
	case "json":
		payload := make([]exportAgentRow, 0, len(rows))
		for _, row := range rows {
			payload = append(payload, exportAgentRow{
				AgentName: row.AgentName,
				Calls:     row.Calls,
				Success:   row.Success,
			})
		}
		return writeJSON(w, payload)
	case "csv":
		records := make([][]string, 0, len(rows))
		for _, row := range rows {
			records = append(records, []string{
				row.AgentName,
				strconv.FormatInt(row.Calls, 10),
				strconv.FormatInt(row.Success, 10),
			})
		}
		return writeCSV(w, []string{"agent_name", "calls", "success"}, records)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeStatsExport(w io.Writer, format string, stats storage.Stats) error {
	switch format {
	case "json":
		return writeJSON(w, exportStatsRow{
			Calls:        stats.Calls,
			SuccessRate:  stats.SuccessRate,
			AvgLatencyMS: stats.AvgLatency,
			InputSize:    stats.InputSize,
			OutputSize:   stats.OutputSize,
		})
	case "csv":
		return writeCSV(w, []string{"calls", "success_rate", "avg_latency_ms", "input_size", "output_size"}, [][]string{{
			strconv.FormatInt(stats.Calls, 10),
			strconv.FormatFloat(stats.SuccessRate, 'f', 2, 64),
			strconv.FormatFloat(stats.AvgLatency, 'f', 2, 64),
			strconv.FormatInt(stats.InputSize, 10),
			strconv.FormatInt(stats.OutputSize, 10),
		}})
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeDailyStatsExport(w io.Writer, format string, rows []storage.DailyStat) error {
	switch format {
	case "json":
		payload := make([]exportDailyStatRow, 0, len(rows))
		for _, row := range rows {
			payload = append(payload, exportDailyStatRow{
				StatDay:         row.StatDay,
				Calls:           row.Calls,
				Success:         row.Success,
				TotalDurationMS: row.TotalDurationMS,
				InputSize:       row.InputSize,
				OutputSize:      row.OutputSize,
			})
		}
		return writeJSON(w, payload)
	case "csv":
		records := make([][]string, 0, len(rows))
		for _, row := range rows {
			records = append(records, []string{
				row.StatDay,
				strconv.FormatInt(row.Calls, 10),
				strconv.FormatInt(row.Success, 10),
				strconv.FormatInt(row.TotalDurationMS, 10),
				strconv.FormatInt(row.InputSize, 10),
				strconv.FormatInt(row.OutputSize, 10),
			})
		}
		return writeCSV(w, []string{"stat_day", "calls", "success", "total_duration_ms", "input_size", "output_size"}, records)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeMonthlyStatsExport(w io.Writer, format string, rows []storage.MonthlyStat) error {
	switch format {
	case "json":
		payload := make([]exportMonthlyStatRow, 0, len(rows))
		for _, row := range rows {
			payload = append(payload, exportMonthlyStatRow{
				StatMonth:       row.StatMonth,
				Calls:           row.Calls,
				Success:         row.Success,
				TotalDurationMS: row.TotalDurationMS,
				InputSize:       row.InputSize,
				OutputSize:      row.OutputSize,
			})
		}
		return writeJSON(w, payload)
	case "csv":
		records := make([][]string, 0, len(rows))
		for _, row := range rows {
			records = append(records, []string{
				row.StatMonth,
				strconv.FormatInt(row.Calls, 10),
				strconv.FormatInt(row.Success, 10),
				strconv.FormatInt(row.TotalDurationMS, 10),
				strconv.FormatInt(row.InputSize, 10),
				strconv.FormatInt(row.OutputSize, 10),
			})
		}
		return writeCSV(w, []string{"stat_month", "calls", "success", "total_duration_ms", "input_size", "output_size"}, records)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeWeeklyStatsExport(w io.Writer, format string, rows []storage.WeeklyStat) error {
	switch format {
	case "json":
		payload := make([]exportWeeklyStatRow, 0, len(rows))
		for _, row := range rows {
			payload = append(payload, exportWeeklyStatRow{
				StatWeek:        row.StatWeek,
				Calls:           row.Calls,
				Success:         row.Success,
				TotalDurationMS: row.TotalDurationMS,
				InputSize:       row.InputSize,
				OutputSize:      row.OutputSize,
			})
		}
		return writeJSON(w, payload)
	case "csv":
		records := make([][]string, 0, len(rows))
		for _, row := range rows {
			records = append(records, []string{
				row.StatWeek,
				strconv.FormatInt(row.Calls, 10),
				strconv.FormatInt(row.Success, 10),
				strconv.FormatInt(row.TotalDurationMS, 10),
				strconv.FormatInt(row.InputSize, 10),
				strconv.FormatInt(row.OutputSize, 10),
			})
		}
		return writeCSV(w, []string{"stat_week", "calls", "success", "total_duration_ms", "input_size", "output_size"}, records)
	default:
		return fmt.Errorf("unsupported export format %q", format)
	}
}

func writeCSV(w io.Writer, headers []string, records [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, record := range records {
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func writeJSON(w io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func openOutput(defaultWriter io.Writer, outputPath string) (io.Writer, func() error, error) {
	if outputPath == "" {
		return defaultWriter, func() error { return nil }, nil
	}
	file, err := os.Create(outputPath)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func writeStatus(out io.Writer, status state.Status) error {
	payload, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(payload))
	return err
}

func wrapError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", action, err)
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, strings.TrimSpace(`
TraceAI

Usage:
  traceai [--store sqlite|memory] [--db path] top-tools
  traceai [--store sqlite|memory] [--db path] top-functions
  traceai [--store sqlite|memory] [--db path] top-agents
  traceai [--store sqlite|memory] [--db path] stats
  traceai [--store sqlite|memory] [--db path] version
  traceai [--store sqlite|memory] [--db path] report [--limit n] [--catalog path|tool1,tool2] [--trend-days n]
  traceai [--store sqlite|memory] [--db path] dashboard [--addr 127.0.0.1:8080] [--token value]
  traceai [--store sqlite|memory] [--db path] status
  traceai [--store sqlite|memory] [--db path] health
  traceai [--store sqlite|memory] [--db path] metrics [--format text|json]
  traceai [--store sqlite|memory] [--db path] seed-demo
  traceai [--store sqlite|memory] [--db path] export <top-tools|top-functions|top-agents|stats|daily-stats|weekly-stats|monthly-stats> [--format csv|json] [--output path]
`))
}

type exportToolRow struct {
	ToolName string `json:"tool_name"`
	Calls    int64  `json:"calls"`
	Success  int64  `json:"success"`
}

type exportFunctionRow struct {
	FunctionName string `json:"function_name"`
	Calls        int64  `json:"calls"`
	Success      int64  `json:"success"`
}

type exportAgentRow struct {
	AgentName string `json:"agent_name"`
	Calls     int64  `json:"calls"`
	Success   int64  `json:"success"`
}

type exportStatsRow struct {
	Calls        int64   `json:"calls"`
	SuccessRate  float64 `json:"success_rate"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	InputSize    int64   `json:"input_size"`
	OutputSize   int64   `json:"output_size"`
}

type exportDailyStatRow struct {
	StatDay         string `json:"stat_day"`
	Calls           int64  `json:"calls"`
	Success         int64  `json:"success"`
	TotalDurationMS int64  `json:"total_duration_ms"`
	InputSize       int64  `json:"input_size"`
	OutputSize      int64  `json:"output_size"`
}

type exportMonthlyStatRow struct {
	StatMonth       string `json:"stat_month"`
	Calls           int64  `json:"calls"`
	Success         int64  `json:"success"`
	TotalDurationMS int64  `json:"total_duration_ms"`
	InputSize       int64  `json:"input_size"`
	OutputSize      int64  `json:"output_size"`
}

type exportWeeklyStatRow struct {
	StatWeek        string `json:"stat_week"`
	Calls           int64  `json:"calls"`
	Success         int64  `json:"success"`
	TotalDurationMS int64  `json:"total_duration_ms"`
	InputSize       int64  `json:"input_size"`
	OutputSize      int64  `json:"output_size"`
}
