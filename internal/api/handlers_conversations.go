package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"chatty/internal/conversations"
	"chatty/internal/messages"
)

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	includeArchived := parseBool(r.URL.Query().Get("include_archived"))
	limit := parseIntDefault(r.URL.Query().Get("limit"), 0)

	if q != "" {
		list, err := s.conv.Search(ctx, q, includeArchived, limit)
		if err != nil {
			s.log.Error("search failed", slog.String("err", err.Error()))
			respondError(w, http.StatusInternalServerError, "search failed")
			return
		}
		s.json(w, http.StatusOK, map[string]any{"conversations": list, "query": q})
		return
	}

	list, err := s.conv.List(ctx, conversations.ListOptions{IncludeArchived: includeArchived, Limit: limit})
	if err != nil {
		s.log.Error("list conversations failed", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}
	if list == nil {
		list = []*conversations.Conversation{}
	}
	s.json(w, http.StatusOK, map[string]any{"conversations": list})
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var body struct {
		Title string `json:"title"`
	}
	if err := s.decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	conv, err := s.conv.Create(ctx, body.Title)
	if err != nil {
		s.log.Error("create conversation failed", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "failed to create conversation")
		return
	}
	s.hub.Publish(`{"type":"conversation.created","id":"` + conv.ID + `"}`)
	s.json(w, http.StatusCreated, conv)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	conv, err := s.conv.Get(ctx, id)
	if err != nil {
		writeConvError(w, s, err)
		return
	}
	s.json(w, http.StatusOK, conv)
}

func (s *Server) handlePatchConversation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if _, err := s.conv.Get(ctx, id); err != nil {
		writeConvError(w, s, err)
		return
	}
	var body struct {
		Title    *string `json:"title"`
		Archived *bool   `json:"archived"`
	}
	if err := s.decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	changed := false
	if body.Title != nil {
		if err := s.conv.Rename(ctx, id, *body.Title); err != nil {
			if strings.Contains(err.Error(), "empty title") {
				respondError(w, http.StatusBadRequest, "title must not be empty")
				return
			}
			writeConvError(w, s, err)
			return
		}
		changed = true
	}
	if body.Archived != nil {
		if err := s.conv.SetArchived(ctx, id, *body.Archived); err != nil {
			writeConvError(w, s, err)
			return
		}
		changed = true
	}
	if !changed {
		respondError(w, http.StatusBadRequest, "no fields to update")
		return
	}
	conv, err := s.conv.Get(ctx, id)
	if err != nil {
		writeConvError(w, s, err)
		return
	}
	s.json(w, http.StatusOK, conv)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	// Refuse to delete mid-generation.
	if s.engine.IsGenerating(id) {
		respondError(w, http.StatusConflict, "conversation is generating; stop it first")
		return
	}
	if err := s.conv.Delete(ctx, id); err != nil {
		writeConvError(w, s, err)
		return
	}
	s.hub.Publish(`{"type":"conversation.deleted","id":"` + id + `"}`)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if _, err := s.conv.Get(ctx, id); err != nil {
		writeConvError(w, s, err)
		return
	}
	limit := parseIntDefault(r.URL.Query().Get("limit"), 0)
	msgs, err := s.msgs.List(ctx, id, limit)
	if err != nil {
		s.log.Error("list messages failed", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "failed to list messages")
		return
	}
	if msgs == nil {
		msgs = []*messages.Message{}
	}
	s.json(w, http.StatusOK, map[string]any{"messages": msgs})
}

func (s *Server) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if _, err := s.conv.Get(ctx, id); err != nil {
		writeConvError(w, s, err)
		return
	}
	var body struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := s.decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	body.Content = strings.TrimSpace(body.Content)
	if body.Content == "" {
		respondError(w, http.StatusBadRequest, "content is required")
		return
	}
	if len([]rune(body.Content)) > s.cfg.Chat.MaxMessageRunes {
		respondError(w, http.StatusRequestEntityTooLarge, "message too long")
		return
	}
	role := body.Role
	if role == "" {
		role = messages.RoleUser
	}
	if role != messages.RoleUser && role != messages.RoleSystem {
		respondError(w, http.StatusBadRequest, "role must be user or system")
		return
	}
	msg, err := s.msgs.Append(ctx, id, role, body.Content, messages.StatusComplete, "")
	if err != nil {
		s.log.Error("append message failed", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "failed to append message")
		return
	}
	_ = s.conv.Touch(ctx, id)
	s.json(w, http.StatusCreated, msg)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	conv, err := s.conv.Get(ctx, id)
	if err != nil {
		writeConvError(w, s, err)
		return
	}
	msgs, err := s.msgs.List(ctx, id, 0)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to load messages")
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	slug := slugify(conv.Title)
	filename := slug + "." + map[bool]string{true: "json", false: "md"}[format == "json"]

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
		s.json(w, http.StatusOK, map[string]any{
			"conversation": conv,
			"messages":     msgs,
			"exported_at":  time.Now().UTC(),
		})
	default: // markdown
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, filename))
		writeMarkdownExport(w, conv, msgs)
	}
}

func writeMarkdownExport(w interface{ Write([]byte) (int, error) }, conv *conversations.Conversation, msgs []*messages.Message) {
	fmt.Fprintf(w, "# %s\n\n", conv.Title)
	fmt.Fprintf(w, "_Exported %s — %d messages — [Chatty](/)_\n\n", conv.UpdatedAt.Format(time.RFC3339), len(msgs))
	fmt.Fprintf(w, "---\n\n")
	for _, m := range msgs {
		if m.Status == messages.StatusFailed {
			continue
		}
		who := "You"
		if m.Role == messages.RoleAssistant {
			who = "Chatty"
		} else if m.Role == messages.RoleSystem {
			who = "System"
		}
		fmt.Fprintf(w, "### %s · %s\n\n%s\n\n", who, m.CreatedAt.UTC().Format("2006-01-02 15:04"), strings.TrimSpace(m.Content))
	}
}

// ---- helpers --------------------------------------------------------------

func writeConvError(w http.ResponseWriter, s *Server, err error) {
	switch {
	case err == nil:
		return
	case strings.Contains(err.Error(), "not found"):
		respondError(w, http.StatusNotFound, "conversation not found")
	default:
		s.log.Error("conversation error", slog.String("err", err.Error()))
		respondError(w, http.StatusInternalServerError, "conversation operation failed")
	}
}

func parseBool(s string) bool {
	v, _ := strconv.ParseBool(s)
	return v
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if !prevDash {
				b.WriteRune('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "conversation"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}
