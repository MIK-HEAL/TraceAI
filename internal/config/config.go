package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Store         string
	DB            string
	ConfigPath    string
	LogLevel      string
	LogFormat     string
	CaptureMode   string
	RetentionDays int
	MaxDBBytes    int64
}

func Load(fs *flag.FlagSet) *Config {
	cfg := &Config{
		Store:       envOrDefault("TRACEAI_STORE", "sqlite"),
		DB:          envOrDefault("TRACEAI_DB", "traceai.db"),
		ConfigPath:  envOrDefault("TRACEAI_CONFIG", ""),
		LogLevel:    envOrDefault("TRACEAI_LOG_LEVEL", "info"),
		LogFormat:   envOrDefault("TRACEAI_LOG_FORMAT", "text"),
		CaptureMode: envOrDefault("TRACEAI_CAPTURE_MODE", "redacted"),
	}
	if fs != nil {
		fs.StringVar(&cfg.Store, "store", cfg.Store, "storage backend: sqlite or memory")
		fs.StringVar(&cfg.DB, "db", cfg.DB, "sqlite database path")
		fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "config file path")
		fs.StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "log level: debug, info, warn, error")
		fs.StringVar(&cfg.LogFormat, "log-format", cfg.LogFormat, "log format: text or json")
		fs.StringVar(&cfg.CaptureMode, "capture-mode", cfg.CaptureMode, "capture mode: metadata-only, redacted, or full-payload")
		fs.IntVar(&cfg.RetentionDays, "retention-days", cfg.RetentionDays, "delete events older than N days (0 disables)")
		fs.Int64Var(&cfg.MaxDBBytes, "max-db-bytes", cfg.MaxDBBytes, "maximum SQLite size after pruning (0 disables)")
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
	if !visited["capture-mode"] && strings.TrimSpace(fileCfg.CaptureMode) != "" {
		cfg.CaptureMode = fileCfg.CaptureMode
	}
	if !visited["retention-days"] {
		cfg.RetentionDays = fileCfg.RetentionDays
	}
	if !visited["max-db-bytes"] {
		cfg.MaxDBBytes = fileCfg.MaxDBBytes
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
	c.CaptureMode = strings.ToLower(strings.TrimSpace(c.CaptureMode))
	if c.CaptureMode == "" {
		c.CaptureMode = "redacted"
	}
	switch c.CaptureMode {
	case "metadata-only", "redacted", "full-payload":
	default:
		return fmt.Errorf("unsupported capture mode %q", c.CaptureMode)
	}
	if c.RetentionDays < 0 {
		return fmt.Errorf("retention days must not be negative")
	}
	if c.MaxDBBytes < 0 {
		return fmt.Errorf("max db bytes must not be negative")
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
