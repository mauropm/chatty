package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"chatty/internal/settings"
)

// GET /api/settings — effective generation settings + safe system info.
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	gen, err := s.settings.Get(ctx)
	if err != nil {
		s.log.Error("read settings failed", slog.String("err", err.Error()))
		gen = s.settings.Defaults()
	}

	dbStatus := "unknown"
	if _, err := s.db.Ping(ctx); err == nil {
		dbStatus = "connected"
	} else {
		dbStatus = "unreachable"
	}

	s.json(w, http.StatusOK, map[string]any{
		"generation": gen,
		"defaults":   s.settings.Defaults(),
		"model": map[string]any{
			"id":       s.mimir.Model(),
			"backend":  string(s.mimir.Backend()),
			"context":  s.mimir.ContextLen(),
			"base_url": s.cfg.Mimir.BaseURL,
		},
		"system": map[string]any{
			"name":            s.cfg.App.Name,
			"version":         s.cfg.App.Version,
			"env":             s.cfg.App.Env,
			"uptime_seconds":  int64(time.Since(s.started).Seconds()),
			"active_model":    s.mimir.Model(),
		},
		"database": map[string]any{"status": dbStatus},
		"mimir":    map[string]any{"status": s.mimirStatus(ctx)},
	})
}

// PATCH /api/settings — validated partial override of generation settings.
func (s *Server) handlePatchSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var patch settings.PartialGeneration
	if err := s.decodeJSON(r, &patch); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	updated, err := s.settings.Patch(ctx, patch)
	if err != nil {
		if isValidation(err) {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.log.Error("patch settings failed", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}
	s.json(w, http.StatusOK, map[string]any{"generation": updated})
}

func (s *Server) effectiveThinking(ctx context.Context) bool {
	gen, err := s.settings.Get(ctx)
	if err != nil {
		return s.cfg.Mimir.Thinking
	}
	return gen.Thinking
}

func (s *Server) mimirStatus(ctx context.Context) string {
	if _, _, err := s.mimir.Health(ctx); err != nil {
		return "unreachable"
	}
	return "connected"
}

func isValidation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return stringsContains(msg, "out of range") || stringsContains(msg, "settings:")
}

func stringsContains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
