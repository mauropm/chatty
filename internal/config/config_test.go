package config

import (
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(env(map[string]string{}))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if cfg.App.Port != 3333 {
		t.Errorf("App.Port = %d, want 3333", cfg.App.Port)
	}
	if cfg.Database.Host != "chatty-db" {
		t.Errorf("Database.Host = %q, want chatty-db", cfg.Database.Host)
	}
	if cfg.Mimir.BaseURL != "http://inference:8080/v1" {
		t.Errorf("Mimir.BaseURL = %q", cfg.Mimir.BaseURL)
	}
	if cfg.Chat.Temperature != 0.7 {
		t.Errorf("Chat.Temperature = %v, want 0.7", cfg.Chat.Temperature)
	}
	if cfg.Mimir.Thinking {
		t.Errorf("Mimir.Thinking = true, want false by default")
	}
	if !cfg.Mimir.Stream {
		t.Errorf("Mimir.Stream = false, want true by default")
	}
}

func TestParseMimirTimeoutBareNumberMeansSeconds(t *testing.T) {
	cfg, err := Parse(env(map[string]string{"MIMIR_TIMEOUT": "300"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mimir.Timeout != 300*time.Second {
		t.Errorf("Mimir.Timeout = %v, want 300s", cfg.Mimir.Timeout)
	}
	cfg, err = Parse(env(map[string]string{"MIMIR_TIMEOUT": "45s"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mimir.Timeout != 45*time.Second {
		t.Errorf("Mimir.Timeout = %v, want 45s", cfg.Mimir.Timeout)
	}
}

func TestParseTrimsBaseURLSlash(t *testing.T) {
	cfg, err := Parse(env(map[string]string{"MIMIR_BASE_URL": "http://host:1234/v1/"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Mimir.BaseURL != "http://host:1234/v1" {
		t.Errorf("BaseURL = %q, want trailing slash trimmed", cfg.Mimir.BaseURL)
	}
}

func TestParseBoolsAndFloats(t *testing.T) {
	cfg, err := Parse(env(map[string]string{
		"MIMIR_THINKING":    "true",
		"MIMIR_STREAM":      "false",
		"CHAT_TEMPERATURE":  "1.5",
		"CHAT_MAX_TOKENS":   "512",
		"APP_PORT":          "8080",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Mimir.Thinking {
		t.Error("thinking should be true")
	}
	if cfg.Mimir.Stream {
		t.Error("stream should be false")
	}
	if cfg.Chat.Temperature != 1.5 || cfg.Chat.MaxTokens != 512 || cfg.App.Port != 8080 {
		t.Errorf("override parsing wrong: %+v", cfg.Chat)
	}
}

func TestValidateRejectsBadValues(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{"bad port", map[string]string{"APP_PORT": "0"}},
		{"bad temp high", map[string]string{"CHAT_TEMPERATURE": "3"}},
		{"bad temp low", map[string]string{"CHAT_TEMPERATURE": "-1"}},
		{"bad backend", map[string]string{"MIMIR_BACKEND": "azure"}},
		{"bad baseurl", map[string]string{"MIMIR_BASE_URL": "ftp://x"}},
		{"zero max tokens", map[string]string{"CHAT_MAX_TOKENS": "0"}},
		{"bad db port", map[string]string{"DATABASE_PORT": "99999"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Parse(env(tc.env)); err == nil {
				t.Fatalf("expected error for %v, got nil", tc.env)
			}
		})
	}
}

func TestDSNContainsParameters(t *testing.T) {
	cfg, _ := Parse(env(map[string]string{"DATABASE_PASSWORD": "s3cr3t"}))
	dsn := cfg.Database.DSN()
	for _, want := range []string{"host=chatty-db", "port=5432", "dbname=chatty", "user=chatty", "sslmode=disable"} {
		if !contains(dsn, want) {
			t.Errorf("DSN %q missing %q", dsn, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
