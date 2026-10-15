package storage

import (
	"fmt"
	"strings"
)

type Config struct {
	Backend       string
	Path          string
	CaptureMode   CaptureMode
	RetentionDays int
	MaxBytes      int64
}

func New(cfg Config) (Storage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "sqlite":
		return NewSQLiteStorageWithOptions(cfg.Path, SQLiteOptions{CaptureMode: cfg.CaptureMode, RetentionDays: cfg.RetentionDays, MaxBytes: cfg.MaxBytes}), nil
	case "memory":
		return NewMemoryStorage(), nil
	default:
		return nil, fmt.Errorf("unknown storage backend %q", cfg.Backend)
	}
}
