// Package config loads and validates Chatty's runtime configuration from the
// environment. Environment variables are the primary source of deployment
// configuration; the values here are strictly typed and validated at startup so
// that the rest of the application can trust them.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved application configuration.
type Config struct {
	App      AppConfig
	Database DatabaseConfig
	Mimir    MimirConfig
	Chat     ChatConfig
}

// AppConfig holds general application settings.
type AppConfig struct {
	Name    string
	Version string
	Env     string // "production" | "development" | "test"
	Port    int    // listen port (IPv4 0.0.0.0)
}

// Addr returns the listen address for the HTTP server.
func (a AppConfig) Addr() string { return fmt.Sprintf("0.0.0.0:%d", a.Port) }

// DatabaseConfig configures the PostgreSQL connection.
type DatabaseConfig struct {
	Host         string
	Port         int
	Name         string
	User         string
	Password     string
	SSLMode      string
	MaxConns     int32
	MinConns     int32
	PingTimeout  time.Duration
}

// DSN builds a pgx/Postgres connection string using parameterized keywords.
func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s pool_max_conns=%d pool_min_conns=%d",
		d.Host, d.Port, d.Name, d.User, d.Password, d.SSLMode, d.MaxConns, d.MinConns,
	)
}

// MimirConfig configures the upstream inference service (llama.cpp/OpenAI-compatible).
type MimirConfig struct {
	BaseURL    string
	Model      string // empty or "auto" => discovered from /v1/models
	APIKey     string
	Timeout    time.Duration
	Stream     bool
	Thinking   bool
	Backend    string // "auto" | "llamacpp" | "openai" — request compatibility selector
}

// ChatConfig configures the generation defaults and context management.
type ChatConfig struct {
	Temperature        float64
	MaxTokens          int
	MaxContextMessages int
	MaxContextTokens   int // budget reserved for the prompt (history trim target)
	SystemPrompt       string
	MaxMessageRunes    int // hard cap on a single submitted message
}

// DefaultSystemPrompt is prepended to every conversation.
const DefaultSystemPrompt = "You are Chatty, a helpful, precise and friendly AI assistant. " +
	"Answer directly and clearly. Use Markdown when it improves readability, and format code in fenced code blocks."

// Load reads configuration from the process environment, applying defaults,
// and validates the result.
func Load() (*Config, error) { return Parse(getenv) }

// Parse builds a Config from an arbitrary environment lookup function, which
// makes configuration unit-testable without mutating the process environment.
func Parse(getenv func(string) string) (*Config, error) {
	cfg := &Config{}

	// ---- App ----
	cfg.App.Name = str(getenv, "APP_NAME", "chatty")
	cfg.App.Version = str(getenv, "APP_VERSION", "1.0.0")
	cfg.App.Env = strings.ToLower(str(getenv, "APP_ENV", "production"))
	cfg.App.Port = intOr(getenv, "APP_PORT", 3333)

	// ---- Database ----
	cfg.Database.Host = str(getenv, "DATABASE_HOST", "chatty-db")
	cfg.Database.Port = intOr(getenv, "DATABASE_PORT", 5432)
	cfg.Database.Name = str(getenv, "DATABASE_NAME", "chatty")
	cfg.Database.User = str(getenv, "DATABASE_USER", "chatty")
	cfg.Database.Password = str(getenv, "DATABASE_PASSWORD", "")
	cfg.Database.SSLMode = str(getenv, "DATABASE_SSLMODE", "disable")
	cfg.Database.MaxConns = int32Or(getenv, "DATABASE_MAX_CONNS", 10)
	cfg.Database.MinConns = int32Or(getenv, "DATABASE_MIN_CONNS", 1)
	cfg.Database.PingTimeout = durOr(getenv, "DATABASE_PING_TIMEOUT", 5*time.Second)

	// ---- Mimir ----
	cfg.Mimir.BaseURL = strings.TrimRight(str(getenv, "MIMIR_BASE_URL", "http://inference:8080/v1"), "/")
	cfg.Mimir.Model = str(getenv, "MIMIR_MODEL", "")
	cfg.Mimir.APIKey = str(getenv, "MIMIR_API_KEY", "")
	cfg.Mimir.Timeout = durOr(getenv, "MIMIR_TIMEOUT", 300*time.Second)
	cfg.Mimir.Stream = boolOr(getenv, "MIMIR_STREAM", true)
	cfg.Mimir.Thinking = boolOr(getenv, "MIMIR_THINKING", false)
	cfg.Mimir.Backend = strings.ToLower(str(getenv, "MIMIR_BACKEND", "auto"))

	// ---- Chat ----
	cfg.Chat.Temperature = floatOr(getenv, "CHAT_TEMPERATURE", 0.7)
	cfg.Chat.MaxTokens = intOr(getenv, "CHAT_MAX_TOKENS", 1024)
	cfg.Chat.MaxContextMessages = intOr(getenv, "CHAT_MAX_CONTEXT_MESSAGES", 40)
	cfg.Chat.MaxContextTokens = intOr(getenv, "CHAT_MAX_CONTEXT_TOKENS", 1400)
	cfg.Chat.SystemPrompt = str(getenv, "CHAT_SYSTEM_PROMPT", DefaultSystemPrompt)
	cfg.Chat.MaxMessageRunes = intOr(getenv, "CHAT_MAX_MESSAGE_RUNES", 32768)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate enforces invariants the rest of the application relies on.
func (c *Config) Validate() error {
	if c.App.Port < 1 || c.App.Port > 65535 {
		return fmt.Errorf("config: APP_PORT out of range: %d", c.App.Port)
	}
	if c.Database.Host == "" {
		return fmt.Errorf("config: DATABASE_HOST is required")
	}
	if c.Database.Name == "" || c.Database.User == "" {
		return fmt.Errorf("config: DATABASE_NAME and DATABASE_USER are required")
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return fmt.Errorf("config: DATABASE_PORT out of range: %d", c.Database.Port)
	}
	if !strings.HasPrefix(c.Mimir.BaseURL, "http://") && !strings.HasPrefix(c.Mimir.BaseURL, "https://") {
		return fmt.Errorf("config: MIMIR_BASE_URL must be an http(s) URL")
	}
	if c.Mimir.Timeout <= 0 {
		return fmt.Errorf("config: MIMIR_TIMEOUT must be positive")
	}
	switch c.Mimir.Backend {
	case "auto", "llamacpp", "openai":
	default:
		return fmt.Errorf("config: MIMIR_BACKEND must be auto|llamacpp|openai, got %q", c.Mimir.Backend)
	}
	if c.Chat.Temperature < 0 || c.Chat.Temperature > 2 {
		return fmt.Errorf("config: CHAT_TEMPERATURE must be within [0,2], got %v", c.Chat.Temperature)
	}
	if c.Chat.MaxTokens < 1 {
		return fmt.Errorf("config: CHAT_MAX_TOKENS must be >= 1")
	}
	if c.Chat.MaxContextMessages < 1 {
		return fmt.Errorf("config: CHAT_MAX_CONTEXT_MESSAGES must be >= 1")
	}
	if c.Chat.MaxContextTokens < 64 {
		return fmt.Errorf("config: CHAT_MAX_CONTEXT_TOKENS must be >= 64")
	}
	if c.Chat.MaxMessageRunes < 1 {
		return fmt.Errorf("config: CHAT_MAX_MESSAGE_RUNES must be >= 1")
	}
	return nil
}

// IsProduction reports whether the app runs in production mode.
func (c *Config) IsProduction() bool { return c.App.Env == "production" }

// ---- environment helpers ----

func getenv(key string) string { return os.Getenv(key) }

func str(get func(string) string, key, def string) string {
	if v := strings.TrimSpace(get(key)); v != "" {
		return v
	}
	return def
}

func intOr(get func(string) string, key string, def int) int {
	if v := strings.TrimSpace(get(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func int32Or(get func(string) string, key string, def int32) int32 {
	return int32(intOr(get, key, int(def)))
}

func floatOr(get func(string) string, key string, def float64) float64 {
	if v := strings.TrimSpace(get(key)); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return def
}

func boolOr(get func(string) string, key string, def bool) bool {
	if v := strings.TrimSpace(get(key)); v != "" {
		if b, err := strconv.ParseBool(strings.ToLower(v)); err == nil {
			return b
		}
	}
	return def
}

func durOr(get func(string) string, key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(get(key))
	if v == "" {
		return def
	}
	// Allow a bare number to mean seconds (e.g. MIMIR_TIMEOUT=300).
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	return def
}
