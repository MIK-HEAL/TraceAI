package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Store      string
	DB         string
	ConfigPath string
}

func Load(fs *flag.FlagSet) *Config {
	cfg := &Config{
		Store:      envOrDefault("TRACEAI_STORE", "sqlite"),
		DB:         envOrDefault("TRACEAI_DB", "toollens.db"),
		ConfigPath: envOrDefault("TRACEAI_CONFIG", ""),
	}
	if fs != nil {
		fs.StringVar(&cfg.Store, "store", cfg.Store, "storage backend: sqlite or memory")
		fs.StringVar(&cfg.DB, "db", cfg.DB, "sqlite database path")
		fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "config file path")
	}
	return cfg
}

func ApplyFile(fs *flag.FlagSet, cfg *Config) error {
	if cfg == nil || cfg.ConfigPath == "" {
		return nil
	}
	data, err := os.ReadFile(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("read config file %q: %w", cfg.ConfigPath, err)
	}
	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return fmt.Errorf("parse config file %q: %w", cfg.ConfigPath, err)
	}
	visited := map[string]bool{}
	if fs != nil {
		fs.Visit(func(f *flag.Flag) {
			visited[f.Name] = true
		})
	}
	if !visited["store"] && strings.TrimSpace(fileCfg.Store) != "" {
		cfg.Store = fileCfg.Store
	}
	if !visited["db"] && strings.TrimSpace(fileCfg.DB) != "" {
		cfg.DB = fileCfg.DB
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
