package api

import (
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatty/internal/chat"
)

func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	var body struct {
		Content string `json:"content"`
	}
	if err := s.decodeJSON(r, &body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	content := strings.TrimSpace(body.Content)
	if len([]rune(content)) > s.cfg.Chat.MaxMessageRunes {
		respondError(w, http.StatusRequestEntityTooLarge, "message too long")
		return
	}
	// Reject concurrent generation before opening the stream.
	if s.engine.IsGenerating(id) {
		respondError(w, http.StatusConflict, "a generation is already running in this conversation")
		return
	}

	sw, ok := newSSE(w)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	if content == "" {
		// Allow resuming generation from existing history only when the last
		// persisted turn is a user message.
		if last, err := s.msgs.LastUserMessage(r.Context(), id); err != nil || last == nil {
			respondError(w, http.StatusBadRequest, "content is required")
			return
		}
	}

	stream := newSafeSSE(sw)
	stop := stream.startHeartbeat()
	defer stop()

	emit := func(ev chat.Event) error { return stream.event(ev.Type, ev.Data) }

	_, err := s.engine.Generate(r.Context(), id, chat.GenerateOptions{Content: content}, emit)
	if err != nil {
		s.log.Debug("generate ended",
			slog.String("conversation_id", id),
			slog.String("err", err.Error()),
		)
	}
	stream.close()
}

func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if s.engine.IsGenerating(id) {
		respondError(w, http.StatusConflict, "a generation is already running in this conversation")
		return
	}
	sw, ok := newSSE(w)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	stream := newSafeSSE(sw)
	stop := stream.startHeartbeat()
	defer stop()

	emit := func(ev chat.Event) error { return stream.event(ev.Type, ev.Data) }
	_, err := s.engine.Generate(r.Context(), id, chat.GenerateOptions{Regenerate: true}, emit)
	if err != nil {
		s.log.Debug("regenerate ended", slog.String("conversation_id", id), slog.String("err", err.Error()))
	}
	stream.close()
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	id := s.pathID(r)
	if id == "" {
		respondError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	stopped := s.engine.Stop(id)
	s.json(w, http.StatusOK, map[string]any{"stopped": stopped})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	sw, ok := newSSE(w)
	if !ok {
		respondError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	stream := newSafeSSE(sw)
	sub := s.hub.Subscribe()
	defer s.hub.Unsubscribe(sub)

	_ = stream.event("hello", map[string]any{
		"service":  s.cfg.App.Name,
		"version":  s.cfg.App.Version,
		"model":    s.mimir.Model(),
		"active":   s.engine.ActiveCount(),
		"server":   time.Now().UTC(),
	})

	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub:
			if _, err := sw.w.Write([]byte("data: " + ev + "\n\n")); err != nil {
				return
			}
			sw.flusher.Flush()
		case <-ticker.C:
			if err := stream.comment("ping"); err != nil {
				return
			}
		}
	}
}

// safeSSE serializes writes to a single SSE connection so a heartbeat goroutine
// can run alongside the generation goroutine.
type safeSSE struct {
	sw *sseWriter
	mu sync.Mutex
}

func newSafeSSE(sw *sseWriter) *safeSSE { return &safeSSE{sw: sw} }

func (w *safeSSE) event(name string, data any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sw.event(name, data)
}

func (w *safeSSE) comment(text string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.sw.comment(text)
	return nil
}

func (w *safeSSE) startHeartbeat() func() {
	done := make(chan struct{})
	var once sync.Once
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				_ = w.comment("keep-alive")
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

func (w *safeSSE) close() {}
