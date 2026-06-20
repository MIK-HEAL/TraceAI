package storage

import (
	"fmt"
	"strings"
)

type Config struct {
	Backend string
	Path    string
}

func New(cfg Config) (Storage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "sqlite":
		return NewSQLiteStorage(cfg.Path), nil
	case "memory":
		return NewMemoryStorage(), nil
	default:
		return nil, fmt.Errorf("unknown storage backend %q", cfg.Backend)
	}
}
