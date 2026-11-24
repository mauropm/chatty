// Package messages persists and retrieves chat messages for conversations.
package messages

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Roles.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Status lifecycle of a message.
const (
	StatusPending   = "pending"
	StatusStreaming = "streaming"
	StatusComplete  = "complete"
	StatusFailed    = "failed"
	StatusStopped   = "stopped"
)

// Message is a stored chat message. IDs are canonical text UUIDs; the database
// stores them as native uuid. Zero-value time fields mean "unset".
type Message struct {
	ID               string         `json:"id"`
	ConversationID   string         `json:"conversation_id"`
	Role             string         `json:"role"`
	Content          string         `json:"content"`
	Status           string         `json:"status"`
	Model            string         `json:"model,omitempty"`
	PromptTokens     int64          `json:"prompt_tokens"`
	CompletionTokens int64          `json:"completion_tokens"`
	LatencyMs        int64          `json:"latency_ms"`
	Error            string         `json:"error,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// Repo provides message persistence.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const msgCols = `id::text, conversation_id::text, role, content, status,
	model, prompt_tokens, completion_tokens, latency_ms, error,
	created_at, completed_at, metadata`

func scanMessage(row pgx.Row) (*Message, error) {
	var (
		m   Message
		mod pgtype.Text
		err pgtype.Text
		comp pgtype.Timestamptz
		meta []byte
	)
	errScan := row.Scan(
		&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Status,
		&mod, &m.PromptTokens, &m.CompletionTokens, &m.LatencyMs, &err,
		&m.CreatedAt, &comp, &meta,
	)
	if errScan != nil {
		return nil, errScan
	}
	if mod.Valid {
		m.Model = mod.String
	}
	if err.Valid {
		m.Error = err.String
	}
	if comp.Valid {
		t := comp.Time
		m.CompletedAt = &t
	}
	if len(meta) > 0 {
		_ = unmarshalJSON(meta, &m.Metadata)
	}
	return &m, nil
}

func unmarshalJSON(b []byte, dst *map[string]any) error {
	if len(b) == 0 {
		return nil
	}
	return jsonUnmarshal(b, dst)
}

// Append inserts a message with the given fields and returns it.
func (r *Repo) Append(ctx context.Context, conversationID, role, content, status, model string) (*Message, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO messages (conversation_id, role, content, status, model)
		VALUES ($1::uuid, $2, $3, $4, NULLIF($5,''))
		RETURNING `+msgCols, conversationID, role, content, status, model)
	return scanMessage(row)
}

// AppendAssistantPending inserts an assistant message in the streaming state so
// that a partial can be persisted if generation is interrupted.
func (r *Repo) AppendAssistantPending(ctx context.Context, conversationID, model string) (*Message, error) {
	return r.Append(ctx, conversationID, RoleAssistant, "", StatusStreaming, model)
}

// UpdateContent persists (partial or final) content for a message, used to
// durably checkpoint a streaming response.
func (r *Repo) UpdateContent(ctx context.Context, id, content, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE messages SET content = $2, status = $3 WHERE id = $1::uuid`, id, content, status)
	return err
}

// FinalizeOpts describes the terminal state of a message.
type FinalizeOpts struct {
	Content          string
	Status           string
	Model            string
	PromptTokens     int64
	CompletionTokens int64
	LatencyMs        int64
	Error            string
}

// Finalize writes the terminal state (content, status, usage, timestamps).
func (r *Repo) Finalize(ctx context.Context, id string, o FinalizeOpts) (*Message, error) {
	var completed any
	if o.Status == StatusComplete || o.Status == StatusStopped || o.Status == StatusFailed {
		completed = time.Now().UTC()
	}
	row := r.pool.QueryRow(ctx, `
		UPDATE messages SET
			content = $2,
			status = $3,
			model = COALESCE(NULLIF($4,''), model),
			prompt_tokens = $5,
			completion_tokens = $6,
			latency_ms = $7,
			error = NULLIF($8,''),
			completed_at = COALESCE($9::timestamptz, completed_at)
		WHERE id = $1::uuid
		RETURNING `+msgCols,
		id, o.Content, o.Status, o.Model, o.PromptTokens, o.CompletionTokens, o.LatencyMs, o.Error, completed)
	return scanMessage(row)
}

// Get loads one message by id.
func (r *Repo) Get(ctx context.Context, id string) (*Message, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+msgCols+` FROM messages WHERE id = $1::uuid`, id)
	m, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return m, err
}

// List returns up to limit messages for a conversation in chronological order.
// A limit <= 0 returns all.
func (r *Repo) List(ctx context.Context, conversationID string, limit int) ([]*Message, error) {
	q := `SELECT ` + msgCols + ` FROM messages WHERE conversation_id = $1::uuid ORDER BY created_at ASC, id ASC`
	args := []any{conversationID}
	if limit > 0 {
		q += ` LIMIT $2`
		args = append(args, limit)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMessages(rows)
}

// ListForContext returns the most recent limit completed user/assistant
// messages in chronological order, suitable for building a model context.
// Failed, pending and streaming messages are excluded so incomplete turns are
// never replayed to the model.
func (r *Repo) ListForContext(ctx context.Context, conversationID string, limit int) ([]*Message, error) {
	if limit <= 0 {
		limit = 1
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+msgCols+` FROM (
			SELECT * FROM messages
			WHERE conversation_id = $1::uuid
			  AND role IN ('user','assistant')
			  AND status = 'complete'
			ORDER BY created_at DESC, id DESC
			LIMIT $2
		) sub
		ORDER BY created_at ASC, id ASC`, conversationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectMessages(rows)
}

// LastUserMessage returns the most recent complete user message, or nil.
func (r *Repo) LastUserMessage(ctx context.Context, conversationID string) (*Message, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+msgCols+` FROM messages
		WHERE conversation_id = $1::uuid AND role = 'user' AND status = 'complete'
		ORDER BY created_at DESC, id DESC LIMIT 1`, conversationID)
	m, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

// LastAssistant returns the most recent assistant message of any status, or nil.
func (r *Repo) LastAssistant(ctx context.Context, conversationID string) (*Message, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+msgCols+` FROM messages
		WHERE conversation_id = $1::uuid AND role = 'assistant'
		ORDER BY created_at DESC, id DESC LIMIT 1`, conversationID)
	m, err := scanMessage(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

// DeleteAssistantAfter removes assistant messages created at or after the given
// time; used by regenerate to drop the message being replaced.
func (r *Repo) DeleteMessage(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM messages WHERE id = $1::uuid`, id)
	return err
}

// Count returns the number of messages in a conversation.
func (r *Repo) Count(ctx context.Context, conversationID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx,
		`SELECT count(*) FROM messages WHERE conversation_id = $1::uuid`, conversationID).Scan(&n)
	return n, err
}

// MarkIncompleteAsFailed transitions any leftover pending/streaming assistant
// messages (e.g. after a crash) to failed. Returns rows affected.
func (r *Repo) MarkIncompleteAsFailed(ctx context.Context) (int64, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE messages SET status = 'failed',
			error = COALESCE(NULLIF(error,''), 'interrupted before completion'),
			completed_at = now()
		WHERE status IN ('pending','streaming')`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func collectMessages(rows pgx.Rows) ([]*Message, error) {
	var out []*Message
	for rows.Next() {
		m, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
