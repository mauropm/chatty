// Package observability provides structured JSON logging and request-context
// helpers used across Chatty.
package observability

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

type ctxKey int

const requestIDKey ctxKey = 1

// NewLogger builds a JSON logger. Levels: debug|info|warn|error.
func NewLogger(level, env string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		if env == "development" || env == "test" {
			lvl = slog.LevelDebug
		} else {
			lvl = slog.LevelInfo
		}
	}
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})
	return slog.New(h).With(slog.String("service", "chatty"))
}

// WithRequestID stores a request ID in the context.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// RequestID returns the request ID stored in the context, or "".
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// LoggerFrom returns a logger decorated with the context's request ID.
func LoggerFrom(ctx context.Context, base *slog.Logger) *slog.Logger {
	if id := RequestID(ctx); id != "" {
		return base.With(slog.String("request_id", id))
	}
	return base
}

// SensitiveKeys are never logged with their real values.
var sensitiveKeys = []string{"api_key", "apikey", "password", "secret", "token", "authorization"}

// Redact returns a copy of a map with sensitive keys masked. Used when logging
// configuration or upstream request payloads.
func Redact(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		lk := strings.ToLower(k)
		redact := false
		for _, s := range sensitiveKeys {
			if strings.Contains(lk, s) {
				redact = true
				break
			}
		}
		if redact {
			if v == "" {
				out[k] = ""
			} else {
				out[k] = "***"
			}
		} else {
			out[k] = v
		}
	}
	return out
}
