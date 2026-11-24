// Package settings stores user-tunable runtime settings in the
// application_settings table. Environment variables remain the source of truth
// for infrastructure configuration; these settings only override a small,
// validated subset of generation parameters at runtime.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"chatty/internal/config"
)

// generationKey is the settings row that stores tunable generation params.
const generationKey = "generation"

// Generation holds tunable generation parameters.
type Generation struct {
	Temperature        float64 `json:"temperature"`
	MaxTokens          int     `json:"max_tokens"`
	Thinking           bool    `json:"thinking"`
	MaxContextMessages int     `json:"max_context_messages"`
	MaxContextTokens   int     `json:"max_context_tokens"`
}

// Bounds for validation.
const (
	minTemp, maxTemp   = 0.0, 2.0
	minMaxTokens       = 1
	maxMaxTokens       = 8192
	minCtxMessages     = 1
	maxCtxMessages     = 200
	minCtxTokens       = 64
	maxCtxTokens       = 16384
)

// Service reads and writes tunable settings.
type Service struct {
	pool     *pgxpool.Pool
	defaults Generation
}

// NewService builds a settings service seeded with environment-derived defaults.
func NewService(pool *pgxpool.Pool, cfg *config.Config) *Service {
	return &Service{
		pool: pool,
		defaults: Generation{
			Temperature:        cfg.Chat.Temperature,
			MaxTokens:          cfg.Chat.MaxTokens,
			Thinking:           cfg.Mimir.Thinking,
			MaxContextMessages: cfg.Chat.MaxContextMessages,
			MaxContextTokens:   cfg.Chat.MaxContextTokens,
		},
	}
}

// Defaults returns the environment-derived defaults.
func (s *Service) Defaults() Generation { return s.defaults }

// Get returns the effective generation settings (defaults + any stored override).
func (s *Service) Get(ctx context.Context) (Generation, error) {
	eff := s.defaults
	raw, err := s.getRaw(ctx, generationKey)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return eff, nil
		}
		return eff, err
	}
	var stored Generation
	if err := json.Unmarshal(raw, &stored); err != nil {
		return eff, fmt.Errorf("settings: decode generation: %w", err)
	}
	mergeGeneration(&eff, stored)
	return eff, nil
}

// Patch applies a partial override and returns the new effective settings.
// Only non-nil fields in patch are applied.
func (s *Service) Patch(ctx context.Context, patch PartialGeneration) (Generation, error) {
	eff, err := s.Get(ctx)
	if err != nil {
		return eff, err
	}
	if patch.Temperature != nil {
		eff.Temperature = *patch.Temperature
	}
	if patch.MaxTokens != nil {
		eff.MaxTokens = *patch.MaxTokens
	}
	if patch.Thinking != nil {
		eff.Thinking = *patch.Thinking
	}
	if patch.MaxContextMessages != nil {
		eff.MaxContextMessages = *patch.MaxContextMessages
	}
	if patch.MaxContextTokens != nil {
		eff.MaxContextTokens = *patch.MaxContextTokens
	}
	if err := validate(eff); err != nil {
		return eff, err
	}
	b, _ := json.Marshal(eff)
	if err := s.putRaw(ctx, generationKey, b); err != nil {
		return eff, err
	}
	return eff, nil
}

// PartialGeneration carries optional overrides for PATCH requests.
type PartialGeneration struct {
	Temperature        *float64 `json:"temperature"`
	MaxTokens          *int     `json:"max_tokens"`
	Thinking           *bool    `json:"thinking"`
	MaxContextMessages *int     `json:"max_context_messages"`
	MaxContextTokens   *int     `json:"max_context_tokens"`
}

func (s *Service) getRaw(ctx context.Context, key string) ([]byte, error) {
	var b []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM application_settings WHERE key = $1`, key).Scan(&b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Service) putRaw(ctx context.Context, key string, value []byte) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO application_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		key, value)
	return err
}

func mergeGeneration(dst *Generation, src Generation) {
	// A stored override is trusted after validation, but guard against
	// nonsensical stored values defensively.
	if src.Temperature >= minTemp && src.Temperature <= maxTemp {
		dst.Temperature = src.Temperature
	}
	if src.MaxTokens >= minMaxTokens && src.MaxTokens <= maxMaxTokens {
		dst.MaxTokens = src.MaxTokens
	}
	dst.Thinking = src.Thinking
	if src.MaxContextMessages >= minCtxMessages && src.MaxContextMessages <= maxCtxMessages {
		dst.MaxContextMessages = src.MaxContextMessages
	}
	if src.MaxContextTokens >= minCtxTokens && src.MaxContextTokens <= maxCtxTokens {
		dst.MaxContextTokens = src.MaxContextTokens
	}
}

func validate(g Generation) error {
	if g.Temperature < minTemp || g.Temperature > maxTemp {
		return fmt.Errorf("settings: temperature %v out of range [%v,%v]", g.Temperature, minTemp, maxTemp)
	}
	if g.MaxTokens < minMaxTokens || g.MaxTokens > maxMaxTokens {
		return fmt.Errorf("settings: max_tokens %d out of range [%d,%d]", g.MaxTokens, minMaxTokens, maxMaxTokens)
	}
	if g.MaxContextMessages < minCtxMessages || g.MaxContextMessages > maxCtxMessages {
		return fmt.Errorf("settings: max_context_messages %d out of range", g.MaxContextMessages)
	}
	if g.MaxContextTokens < minCtxTokens || g.MaxContextTokens > maxCtxTokens {
		return fmt.Errorf("settings: max_context_tokens %d out of range", g.MaxContextTokens)
	}
	return nil
}
