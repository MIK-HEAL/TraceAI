package main

import (
	"bytes"
	"context"
	"path/filepath"
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
