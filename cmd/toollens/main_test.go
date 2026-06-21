package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"toollens/internal/analytics"
	"toollens/internal/storage"
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

func TestReportCommandOutputsSections(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "report.db")
	var stdout bytes.Buffer
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "seed-demo"}, &stdout); err != nil {
		t.Fatal(err)
	}

	stdout.Reset()
	if err := run([]string{"--store", "sqlite", "--db", dbPath, "report", "--limit", "3"}, &stdout); err != nil {
		t.Fatal(err)
	}
	content := stdout.String()
	if !strings.Contains(content, "Tool Heatmap") {
		t.Fatalf("expected tool heatmap section, got: %s", content)
	}
	if !strings.Contains(content, "Error Rate Ranking") {
		t.Fatalf("expected error rate section, got: %s", content)
	}
	if !strings.Contains(content, "Agent Usage") {
		t.Fatalf("expected agent usage section, got: %s", content)
	}
	if !strings.Contains(content, "search") {
		t.Fatalf("expected report data rows, got: %s", content)
	}
}
