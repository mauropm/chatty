// Package conversations persists and manages chat conversation threads.
package conversations

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNotFound is returned when a conversation does not exist.
var ErrNotFound = errors.New("conversations: not found")

// Conversation is a chat thread.
type Conversation struct {
	ID         string         `json:"id"`
	Title      string         `json:"title"`
	Archived   bool           `json:"archived"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	MessageCount int64        `json:"message_count"`
}

// Repo provides conversation persistence.
type Repo struct{ pool *pgxpool.Pool }

// NewRepo builds a Repo.
func NewRepo(pool *pgxpool.Pool) *Repo { return &Repo{pool: pool} }

const convCols = `c.id::text, c.title, c.archived, c.metadata, c.created_at, c.updated_at,
	COALESCE(mc.n, 0) AS message_count`

const convFrom = `
	FROM conversations c LEFT JOIN (
		SELECT conversation_id, count(*) AS n FROM messages GROUP BY conversation_id
	) mc ON mc.conversation_id = c.id`

func scanConversation(row pgx.Row) (*Conversation, error) {
	var (
		c    Conversation
		meta []byte
	)
	if err := row.Scan(&c.ID, &c.Title, &c.Archived, &meta, &c.CreatedAt, &c.UpdatedAt, &c.MessageCount); err != nil {
		return nil, err
	}
	if len(meta) > 0 {
		_ = unmarshalMeta(meta, &c.Metadata)
	}
	return &c, nil
}

// Create inserts a new conversation with the given title.
func (r *Repo) Create(ctx context.Context, title string) (*Conversation, error) {
	if strings.TrimSpace(title) == "" {
		title = "New conversation"
	}
	var id string
	err := r.pool.QueryRow(ctx,
		`INSERT INTO conversations (title) VALUES ($1) RETURNING id::text`, title).Scan(&id)
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, id)
}

// Get loads one conversation.
func (r *Repo) Get(ctx context.Context, id string) (*Conversation, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+convCols+convFrom+` WHERE c.id = $1::uuid`, id)
	c, err := scanConversation(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, err
}

// ListOptions controls conversation listing.
type ListOptions struct {
	IncludeArchived bool
	Limit           int
}

// List returns conversations ordered by most-recently-updated first.
func (r *Repo) List(ctx context.Context, opt ListOptions) ([]*Conversation, error) {
	where := `WHERE ($1 OR NOT c.archived)`
	args := []any{opt.IncludeArchived}
	q := `SELECT ` + convCols + convFrom + ` ` + where + ` ORDER BY c.updated_at DESC, c.id DESC`
	if opt.Limit > 0 {
		q += ` LIMIT $2`
		args = append(args, opt.Limit)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectConversations(rows)
}

// Rename updates the conversation title.
func (r *Repo) Rename(ctx context.Context, id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("conversations: empty title")
	}
	tag, err := r.pool.Exec(ctx, `UPDATE conversations SET title = $2 WHERE id = $1::uuid`, id, title)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetArchived archives or restores a conversation.
func (r *Repo) SetArchived(ctx context.Context, id string, archived bool) error {
	tag, err := r.pool.Exec(ctx, `UPDATE conversations SET archived = $2 WHERE id = $1::uuid`, id, archived)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a conversation and (via cascade) its messages.
func (r *Repo) Delete(ctx context.Context, id string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM conversations WHERE id = $1::uuid`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Touch bumps updated_at so the conversation bubbles to the top of the list.
func (r *Repo) Touch(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE conversations SET updated_at = now() WHERE id = $1::uuid`, id)
	return err
}

// Search matches conversations by title or by message content, ranked newest
// first. It uses full-text search plus an ILIKE fallback for substrings.
func (r *Repo) Search(ctx context.Context, query string, includeArchived bool, limit int) ([]*Conversation, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []*Conversation{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `
		WITH matched AS (
			SELECT c.id
			FROM conversations c
			WHERE ($3 OR NOT c.archived)
			  AND (
				c.search @@ plainto_tsquery('simple', $1)
				OR c.title ILIKE '%' || $1 || '%'
				OR EXISTS (
					SELECT 1 FROM messages m
					WHERE m.conversation_id = c.id
					  AND (m.search @@ plainto_tsquery('simple', $1) OR m.content ILIKE '%' || $1 || '%')
				)
			  )
		)
		SELECT `+convCols+convFrom+`
		WHERE c.id IN (SELECT id FROM matched)
		ORDER BY c.updated_at DESC
		LIMIT $2`, query, limit, includeArchived)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectConversations(rows)
}

func collectConversations(rows pgx.Rows) ([]*Conversation, error) {
	var out []*Conversation
	for rows.Next() {
		c, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- title generation -----------------------------------------------------

// GenerateTitle derives a concise title from the first user message without an
// extra LLM call. It collapses whitespace and punctuation noise, then truncates
// on a word boundary.
func GenerateTitle(firstUserMessage string) string {
	const maxRunes = 60
	// Normalize whitespace and strip leading list/quote markers.
	cleaned := strings.Builder{}
	prevSpace := true
	for _, r := range firstUserMessage {
		switch {
		case unicode.IsSpace(r):
			if !prevSpace {
				cleaned.WriteRune(' ')
				prevSpace = true
			}
		case r == '#' || r == '>' || r == '`' || r == '*':
			// drop markdown emphasis/marker characters
		default:
			cleaned.WriteRune(r)
			prevSpace = false
		}
	}
	s := strings.TrimSpace(cleaned.String())
	// Drop leading list bullets like "- " that survived.
	s = strings.TrimLeft(s, "-• ")
	if s == "" {
		return "New conversation"
	}
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	// Truncate on a word boundary near the limit.
	cut := maxRunes
	for cut > 0 && runes[cut] != ' ' {
		cut--
	}
	if cut <= 0 {
		cut = maxRunes
	}
	return strings.TrimSpace(string(runes[:cut])) + "…"
}

func unmarshalMeta(b []byte, dst *map[string]any) error {
	if len(b) == 0 {
		return nil
	}
	return jsonUnmarshal(b, dst)
}
