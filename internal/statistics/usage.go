package statistics

import (
	"context"
	"encoding/json"
)

// UsageEvent describes a single upstream generation attempt.
type UsageEvent struct {
	ConversationID   string
	MessageID        string
	EventType        string // defaults to "chat_completion"
	Model            string
	PromptTokens     int64
	CompletionTokens int64
	LatencyMs        int64
	Success          bool
	Metadata         map[string]any
}

// Record inserts a usage event. A failure to record is returned but should not
// abort a generation; callers log and continue.
func (s *Service) RecordUsage(ctx context.Context, ev UsageEvent) error {
	if ev.EventType == "" {
		ev.EventType = "chat_completion"
	}
	meta := []byte("{}")
	if len(ev.Metadata) > 0 {
		if b, err := json.Marshal(ev.Metadata); err == nil {
			meta = b
		}
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO usage_events
			(conversation_id, message_id, event_type, model, prompt_tokens, completion_tokens, latency_ms, success, metadata)
		VALUES
			(NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, $3, NULLIF($4,''), $5, $6, $7, $8, $9::jsonb)`,
		ev.ConversationID, ev.MessageID, ev.EventType, ev.Model,
		ev.PromptTokens, ev.CompletionTokens, ev.LatencyMs, ev.Success, meta)
	return err
}
