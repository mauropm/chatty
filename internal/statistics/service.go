// Package statistics answers health and analytics questions from real
// PostgreSQL data (no counters kept in memory).
package statistics

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Overview is the aggregate counters used by /health.
type Overview struct {
	TotalConversations     int64 `json:"total_conversations"`
	TotalMessages          int64 `json:"total_messages"`
	TotalUserMessages      int64 `json:"total_user_messages"`
	TotalAssistantMessages int64 `json:"total_assistant_messages"`
	TotalTokensPrompt      int64 `json:"total_tokens_prompt"`
	TotalTokensCompletion  int64 `json:"total_tokens_completion"`
	TotalRequests          int64 `json:"total_requests"`
	SuccessfulRequests     int64 `json:"successful_requests"`
	FailedRequests         int64 `json:"failed_requests"`
}

// DailyActivity is one day of activity.
type DailyActivity struct {
	Date             string `json:"date"`
	Messages         int64  `json:"messages"`
	Requests         int64  `json:"requests"`
	Successful       int64  `json:"successful_requests"`
	Failed           int64  `json:"failed_requests"`
	TokensPrompt     int64  `json:"tokens_prompt"`
	TokensCompletion int64  `json:"tokens_completion"`
}

// ModelUsage is usage aggregated per model.
type ModelUsage struct {
	Model           string  `json:"model"`
	Requests        int64   `json:"requests"`
	TokensPrompt    int64   `json:"tokens_prompt"`
	TokensCompletion int64  `json:"tokens_completion"`
	AvgLatencyMs    float64 `json:"avg_latency_ms"`
}

// Report is the full payload returned by /api/stats.
type Report struct {
	Overview
	MessagesByRole map[string]int64  `json:"messages_by_role"`
	ModelUsage     []ModelUsage      `json:"model_usage"`
	Daily7         []DailyActivity   `json:"daily_activity_7d"`
	Daily30        []DailyActivity   `json:"daily_activity_30d"`
	AvgLatencyMs   float64           `json:"avg_latency_ms"`
	GeneratedAt    time.Time         `json:"generated_at"`
}

// Service runs statistics queries.
type Service struct{ pool *pgxpool.Pool }

// NewService builds a statistics service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Overview returns aggregate counters.
func (s *Service) Overview(ctx context.Context) (Overview, error) {
	var o Overview
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM conversations),
			(SELECT count(*) FROM messages),
			(SELECT count(*) FROM messages WHERE role = 'user'),
			(SELECT count(*) FROM messages WHERE role = 'assistant'),
			(SELECT COALESCE(sum(prompt_tokens),0) FROM usage_events),
			(SELECT COALESCE(sum(completion_tokens),0) FROM usage_events),
			(SELECT count(*) FROM usage_events),
			(SELECT count(*) FROM usage_events WHERE success),
			(SELECT count(*) FROM usage_events WHERE NOT success)
	`).Scan(
		&o.TotalConversations, &o.TotalMessages,
		&o.TotalUserMessages, &o.TotalAssistantMessages,
		&o.TotalTokensPrompt, &o.TotalTokensCompletion,
		&o.TotalRequests, &o.SuccessfulRequests, &o.FailedRequests,
	)
	return o, err
}

// MessagesByRole returns counts grouped by role.
func (s *Service) MessagesByRole(ctx context.Context) (map[string]int64, error) {
	rows, err := s.pool.Query(ctx, `SELECT role, count(*) FROM messages GROUP BY role`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var role string
		var n int64
		if err := rows.Scan(&role, &n); err != nil {
			return nil, err
		}
		out[role] = n
	}
	return out, rows.Err()
}

// ModelUsage returns per-model aggregates. days <= 0 means all time.
func (s *Service) ModelUsage(ctx context.Context, days int) ([]ModelUsage, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT
			COALESCE(model,'unknown') AS model,
			count(*) AS requests,
			COALESCE(sum(prompt_tokens),0) AS tp,
			COALESCE(sum(completion_tokens),0) AS tc,
			COALESCE(avg(latency_ms) FILTER (WHERE latency_ms > 0),0)::float8 AS avg_latency
		FROM usage_events
		WHERE ($1::int <= 0 OR created_at >= now() - ($1::int || ' days')::interval)
		GROUP BY 1
		ORDER BY requests DESC`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelUsage
	for rows.Next() {
		var m ModelUsage
		if err := rows.Scan(&m.Model, &m.Requests, &m.TokensPrompt, &m.TokensCompletion, &m.AvgLatencyMs); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// DailyActivity returns a zero-filled series for the last `days` days, oldest
// first. Dates are ISO YYYY-MM-DD in UTC.
func (s *Service) DailyActivity(ctx context.Context, days int) ([]DailyActivity, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := s.pool.Query(ctx, `
		SELECT
			to_char(d.day, 'YYYY-MM-DD') AS day,
			COALESCE(m.messages,0)   AS messages,
			COALESCE(u.requests,0)    AS requests,
			COALESCE(u.successful,0)  AS successful,
			COALESCE(u.failed,0)      AS failed,
			COALESCE(u.tp,0)          AS tp,
			COALESCE(u.tc,0)          AS tc
		FROM generate_series(
			(date_trunc('day', now()) - (($1::int - 1) || ' days')::interval),
			date_trunc('day', now()),
			'1 day'
		) AS d(day)
		LEFT JOIN (
			SELECT date_trunc('day', created_at) AS day, count(*) AS messages
			FROM messages
			WHERE created_at >= date_trunc('day', now()) - (($1::int - 1) || ' days')::interval
			GROUP BY 1
		) m ON m.day = d.day
		LEFT JOIN (
			SELECT date_trunc('day', created_at) AS day,
				count(*) AS requests,
				count(*) FILTER (WHERE success) AS successful,
				count(*) FILTER (WHERE NOT success) AS failed,
				COALESCE(sum(prompt_tokens),0) AS tp,
				COALESCE(sum(completion_tokens),0) AS tc
			FROM usage_events
			WHERE created_at >= date_trunc('day', now()) - (($1::int - 1) || ' days')::interval
			GROUP BY 1
		) u ON u.day = d.day
		ORDER BY d.day ASC`, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyActivity
	for rows.Next() {
		var a DailyActivity
		if err := rows.Scan(&a.Date, &a.Messages, &a.Requests, &a.Successful, &a.Failed, &a.TokensPrompt, &a.TokensCompletion); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AvgLatencyMs returns the mean generation latency across successful events.
func (s *Service) AvgLatencyMs(ctx context.Context) (float64, error) {
	var v float64
	err := s.pool.QueryRow(ctx, `
		SELECT COALESCE(avg(latency_ms) FILTER (WHERE success AND latency_ms > 0),0)::float8
		FROM usage_events`).Scan(&v)
	return v, err
}

// Report builds the complete /api/stats payload.
func (s *Service) Report(ctx context.Context) (*Report, error) {
	ov, err := s.Overview(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.MessagesByRole(ctx)
	if err != nil {
		return nil, err
	}
	models, err := s.ModelUsage(ctx, 30)
	if err != nil {
		return nil, err
	}
	d7, err := s.DailyActivity(ctx, 7)
	if err != nil {
		return nil, err
	}
	d30, err := s.DailyActivity(ctx, 30)
	if err != nil {
		return nil, err
	}
	avg, err := s.AvgLatencyMs(ctx)
	if err != nil {
		return nil, err
	}
	return &Report{
		Overview:       ov,
		MessagesByRole: roles,
		ModelUsage:     models,
		Daily7:         d7,
		Daily30:        d30,
		AvgLatencyMs:   avg,
		GeneratedAt:    time.Now().UTC(),
	}, nil
}
