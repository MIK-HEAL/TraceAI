package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/MIK-HEAL/TraceAI/internal/analytics"
	"github.com/MIK-HEAL/TraceAI/internal/events"
	"github.com/MIK-HEAL/TraceAI/internal/storage"
)

func TestEmptyStorageHasNoSeedData(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	engine := analytics.NewEngine(store)
	rows, err := engine.TopTools(context.Background(), time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected empty query result, got %d rows", len(rows))
	}
}

func TestSeedDemoDataExplicitlyAddsEvents(t *testing.T) {
	store := storage.NewMemoryStorage()
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := seedDemoData(store); err != nil {
		t.Fatal(err)
	}
	events, err := store.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected demo events after explicit seed")
	}
}

func TestSeedDemoCommandWritesData(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "seed-demo.db")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "demo data inserted") {
		t.Fatalf("expected seed confirmation, got %q", stdout.String())
	}

	store := storage.NewSQLiteStorage(dbPath)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	events, err := store.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected events to be written by seed-demo command")
	}
}

func TestConfigFileCanDriveSeedDemo(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "config-demo.db")
	configPath := filepath.Join(tmpDir, "traceai.json")
	payload, err := json.Marshal(struct {
		Store string `json:"store"`
		DB    string `json:"db"`
	}{Store: "sqlite", DB: dbPath})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, payload, 0644); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := run([]string{"--config", configPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}

	store := storage.NewSQLiteStorage(dbPath)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	events, err := store.ListEvents(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected events to be written by config file")
	}
}

func TestExportTopToolsCSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "export.db")
	csvPath := filepath.Join(tmpDir, "top-tools.csv")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "export", "top-tools", "--format", "csv", "--output", csvPath}, &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "tool_name,calls,success") {
		t.Fatalf("unexpected csv export: %s", string(data))
	}
	if strings.Count(strings.TrimSpace(content), "\n") < 2 {
		t.Fatalf("expected csv data rows, got: %s", content)
	}
	if !strings.Contains(content, "search") {
		t.Fatalf("expected exported csv to include data rows, got: %s", content)
	}
}

func TestExportStatsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "export-json.db")
	jsonPath := filepath.Join(tmpDir, "stats.json")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "export", "stats", "--format", "json", "--output", jsonPath}, &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "\"calls\"") {
		t.Fatalf("unexpected json export: %s", string(data))
	}
	if !strings.Contains(content, strconv.Itoa(5)) {
		t.Fatalf("expected json export to contain seeded rows, got: %s", content)
	}
}

func TestExportMonthlyStatsJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "monthly-export.db")
	jsonPath := filepath.Join(tmpDir, "monthly.json")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}

	store := storage.NewSQLiteStorage(dbPath)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	oldEvent := events.NewToolEvent()
	oldEvent.Timestamp = time.Now().UTC().AddDate(0, -1, 0)
	oldEvent.AgentName = "monthly-agent"
	oldEvent.AgentVersion = "1.0.0"
	oldEvent.AdapterName = "mcp"
	oldEvent.AdapterVersion = "1.0.0"
	oldEvent.ToolType = "mcp"
	oldEvent.ToolName = "archive"
	oldEvent.FunctionName = "delete_file"
	oldEvent.Success = true
	oldEvent.DurationMS = 120
	oldEvent.InputSize = 44
	oldEvent.OutputSize = 12
	if err := store.InsertEvent(context.Background(), oldEvent); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"--store", "sqlite", "--db", dbPath, "export", "monthly-stats", "--format", "json", "--output", jsonPath}, &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	currentMonth := time.Now().UTC().Format("2006-01")
	prevMonth := time.Now().UTC().AddDate(0, -1, 0).Format("2006-01")
	if !strings.Contains(content, "\"stat_month\"") {
		t.Fatalf("unexpected monthly export: %s", content)
	}
	if !strings.Contains(content, currentMonth) || !strings.Contains(content, prevMonth) {
		t.Fatalf("expected both month buckets in export, got: %s", content)
	}
}

func TestExportWeeklyStatsCSV(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "weekly.db")
	csvPath := filepath.Join(tmpDir, "weekly.csv")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "export", "weekly-stats", "--format", "csv", "--output", csvPath}, &stdout); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(csvPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "stat_week,calls,success") {
		t.Fatalf("unexpected weekly export: %s", content)
	}
}

func TestReportCommandOutputsSections(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "report.db")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}

	store := storage.NewSQLiteStorage(dbPath)
	if err := store.Init(context.Background()); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(24 * time.Hour)
	for i, row := range []struct {
		when     time.Time
		toolName string
		fnName   string
		success  bool
		message  string
	}{
		{when: now.AddDate(0, 0, -2), toolName: "archive", fnName: "delete_file", success: false, message: "permission denied"},
		{when: now.AddDate(0, 0, -1), toolName: "docs", fnName: "read_file", success: true, message: ""},
	} {
		event := events.NewToolEvent()
		event.Timestamp = row.when.Add(time.Duration(i) * time.Hour)
		event.AgentName = "report-agent"
		event.AgentVersion = "1.0.0"
		event.AdapterName = "mcp"
		event.AdapterVersion = "1.0.0"
		event.ToolType = "mcp"
		event.ToolName = row.toolName
		event.FunctionName = row.fnName
		event.Success = row.success
		event.ErrorMessage = row.message
		if i == 0 {
			event.ErrorType = "permission_error"
			event.ErrorCode = "forbidden"
		}
		event.DurationMS = int64(100 + i*10)
		if err := store.InsertEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	catalogPath := filepath.Join(tmpDir, "catalog.txt")
	if err := os.WriteFile(catalogPath, []byte("search\nread\nwrite\ndelete_branch\narchive\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "report", "--limit", "3", "--catalog", catalogPath, "--trend-days", "7"}, &stdout); err != nil {
		t.Fatal(err)
	}
	content := stdout.String()
	if !strings.Contains(content, "Tool Heatmap") {
		t.Fatalf("expected tool heatmap section, got: %s", content)
	}
	if !strings.Contains(content, "Zero Call Tools") {
		t.Fatalf("expected zero call section, got: %s", content)
	}
	if !strings.Contains(content, "Error Rate Ranking") {
		t.Fatalf("expected error rate section, got: %s", content)
	}
	if !strings.Contains(content, "Behavior Profile") {
		t.Fatalf("expected behavior profile section, got: %s", content)
	}
	if !strings.Contains(content, "Failure Reasons") {
		t.Fatalf("expected failure reasons section, got: %s", content)
	}
	if !strings.Contains(content, "Trend") {
		t.Fatalf("expected trend section, got: %s", content)
	}
	if !strings.Contains(content, "Agent Usage") {
		t.Fatalf("expected agent usage section, got: %s", content)
	}
	if !strings.Contains(content, "delete_branch") {
		t.Fatalf("expected zero call output, got: %s", content)
	}
	if !strings.Contains(content, "delta calls=") {
		t.Fatalf("expected trend delta output, got: %s", content)
	}
	if !strings.Contains(content, "read-heavy") && !strings.Contains(content, "balanced") {
		t.Fatalf("expected behavior profile output, got: %s", content)
	}
}

func TestVersionCommandOutputsVersion(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"--store", "memory", "version"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "version=") {
		t.Fatal("expected version output")
	}
}

func TestVersionCommandDoesNotNeedStorage(t *testing.T) {
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", filepath.Join(t.TempDir(), "broken.db"), "version"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "version=") {
		t.Fatalf("expected version output, got %q", stdout.String())
	}
}

func TestMetricsJSONFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "metrics-json.db")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "metrics", "--format", "json"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "\"name\"") {
		t.Fatalf("expected json metrics output, got: %s", stdout.String())
	}
}

func TestStatusHealthMetricsCommands(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "ops.db")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "status"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "\"storage_ok\": true") {
		t.Fatalf("expected storage status in output, got: %s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "health"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "ok" {
		t.Fatalf("expected health ok, got: %s", stdout.String())
	}

	stdout.Reset()
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "metrics"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "calls=") {
		t.Fatalf("expected metrics output, got: %s", stdout.String())
	}
}
