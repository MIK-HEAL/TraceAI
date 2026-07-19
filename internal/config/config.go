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
	LogLevel   string
	LogFormat  string
}

func Load(fs *flag.FlagSet) *Config {
	cfg := &Config{
		Store:      envOrDefault("TRACEAI_STORE", "sqlite"),
		DB:         envOrDefault("TRACEAI_DB", "traceai.db"),
		ConfigPath: envOrDefault("TRACEAI_CONFIG", ""),
		LogLevel:   envOrDefault("TRACEAI_LOG_LEVEL", "info"),
		LogFormat:  envOrDefault("TRACEAI_LOG_FORMAT", "text"),
	}
	if fs != nil {
		fs.StringVar(&cfg.Store, "store", cfg.Store, "storage backend: sqlite or memory")
		fs.StringVar(&cfg.DB, "db", cfg.DB, "sqlite database path")
		fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "config file path")
		fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, error")
		fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")
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
	if !visited["log-level"] && strings.TrimSpace(fileCfg.LogLevel) != "" {
		cfg.LogLevel = fileCfg.LogLevel
	}
	if !visited["log-format"] && strings.TrimSpace(fileCfg.LogFormat) != "" {
		cfg.LogFormat = fileCfg.LogFormat
	}
	return nil
}

// Validate normalizes accepted aliases and rejects values that would otherwise
// silently fall back to another runtime behaviour.
func (c *Config) Validate() error {
	if c == nil {
		return fmt.Errorf("config is required")
	}
	c.Store = strings.ToLower(strings.TrimSpace(c.Store))
	switch c.Store {
	case "sqlite", "memory":
	default:
		return fmt.Errorf("unsupported store %q (expected sqlite or memory)", c.Store)
	}
	c.DB = strings.TrimSpace(c.DB)
	if c.Store == "sqlite" && c.DB == "" {
		return fmt.Errorf("db path is required for sqlite storage")
	}

	c.LogLevel = strings.ToLower(strings.TrimSpace(c.LogLevel))
	if c.LogLevel == "warning" {
		c.LogLevel = "warn"
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("unsupported log level %q (expected debug, info, warn, or error)", c.LogLevel)
	}

	c.LogFormat = strings.ToLower(strings.TrimSpace(c.LogFormat))
	switch c.LogFormat {
	case "text", "json":
	default:
		return fmt.Errorf("unsupported log format %q (expected text or json)", c.LogFormat)
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
