package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"chatty/internal/observability"
)

// healthResponse mirrors the documented /health contract.
type healthResponse struct {
	Status        string         `json:"status"`
	Service       string         `json:"service"`
	Version       string         `json:"version"`
	UptimeSeconds int64          `json:"uptime_seconds"`
	Database      dependencyStat `json:"database"`
	Mimir         mimirStat      `json:"mimir"`
	Statistics    any            `json:"statistics"`
}

type dependencyStat struct {
	Status    string `json:"status"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type mimirStat struct {
	Status    string `json:"status"`
	Model     string `json:"model"`
	LatencyMs int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	log := observability.LoggerFrom(ctx, s.log)

	resp := healthResponse{
		Service:       s.cfg.App.Name,
		Version:       s.cfg.App.Version,
		UptimeSeconds: int64(time.Since(s.started).Seconds()),
	}

	// Database — critical dependency.
	if lat, err := s.db.Ping(ctx); err != nil {
		resp.Status = "unhealthy"
		resp.Database = dependencyStat{Status: "unreachable", Error: err.Error()}
	} else {
		resp.Database = dependencyStat{Status: "connected", LatencyMs: lat}
	}

	// Mimir — core dependency; degraded (not 503) if it is briefly unavailable
	// so container health checks don't flap while llama-server reloads.
	if lat, model, err := s.mimir.Health(ctx); err != nil {
		resp.Mimir = mimirStat{Status: "unreachable", Model: s.mimir.Model(), Error: err.Error()}
	} else {
		resp.Mimir = mimirStat{Status: "connected", Model: model, LatencyMs: lat}
	}

	// Statistics from PostgreSQL (best-effort).
	if ov, err := s.stats.Overview(ctx); err != nil {
		log.Warn("health stats failed", slog.String("err", err.Error()))
		resp.Statistics = map[string]any{"error": "unavailable"}
	} else {
		resp.Statistics = ov
	}

	switch {
	case resp.Database.Status != "connected":
		s.json(w, http.StatusServiceUnavailable, resp)
	case resp.Mimir.Status != "connected":
		resp.Status = "degraded"
		s.json(w, http.StatusOK, resp)
	default:
		resp.Status = "healthy"
		s.json(w, http.StatusOK, resp)
	}
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	report, err := s.stats.Report(ctx)
	if err != nil {
		s.log.Error("stats failed", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "statistics unavailable")
		return
	}
	s.json(w, http.StatusOK, report)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	info, err := s.mimir.Discover(ctx)
	if err != nil {
		respondError(w, http.StatusBadGateway, "model discovery failed: "+err.Error())
		return
	}
	s.json(w, http.StatusOK, map[string]any{
		"models": []map[string]any{{
			"id":        info.ID,
			"owned_by":  info.OwnedBy,
			"context":   info.ContextLen,
			"active":    info.ID == s.mimir.Model(),
			"backend":   string(s.mimir.Backend()),
			"thinking":  s.effectiveThinking(ctx),
		}},
		"active_model": s.mimir.Model(),
	})
}
