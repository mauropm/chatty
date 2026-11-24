// Package api exposes Chatty's HTTP surface: REST endpoints, SSE streaming,
// static asset serving, health and analytics.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"chatty/internal/chat"
	"chatty/internal/config"
	"chatty/internal/conversations"
	"chatty/internal/database"
	"chatty/internal/messages"
	"chatty/internal/mimir"
	"chatty/internal/observability"
	"chatty/internal/settings"
	"chatty/internal/statistics"
)

// Server holds all HTTP dependencies.
type Server struct {
	cfg      *config.Config
	db       *database.DB
	conv     *conversations.Repo
	msgs     *messages.Repo
	mimir    *mimir.Client
	stats    *statistics.Service
	settings *settings.Service
	engine   *chat.Engine
	log      *slog.Logger
	started  time.Time
	hub      *Hub

	maxBodyBytes int64
}

// Deps wires the server.
type Deps struct {
	Config   *config.Config
	DB       *database.DB
	Conv     *conversations.Repo
	Msgs     *messages.Repo
	Mimir    *mimir.Client
	Stats    *statistics.Service
	Settings *settings.Service
	Engine   *chat.Engine
	Logger   *slog.Logger
}

// New builds a Server.
func New(d Deps) *Server {
	return &Server{
		cfg:          d.Config,
		db:           d.DB,
		conv:         d.Conv,
		msgs:         d.Msgs,
		mimir:        d.Mimir,
		stats:        d.Stats,
		settings:     d.Settings,
		engine:       d.Engine,
		log:          d.Logger,
		started:      time.Now(),
		hub:          NewHub(),
		maxBodyBytes: 1 << 20, // 1 MiB request body cap
	}
}

// Handler builds the full middleware-wrapped route tree.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// App shell + static.
	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.Handle("GET /static/", http.StripPrefix("/static/", s.staticHandler()))
	mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	mux.HandleFunc("GET /manifest.webmanifest", s.handleManifest)

	// Health & analytics.
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/stats", s.handleStats)

	// Conversations.
	mux.HandleFunc("GET /api/conversations", s.handleListConversations)
	mux.HandleFunc("POST /api/conversations", s.handleCreateConversation)
	mux.HandleFunc("GET /api/conversations/{id}", s.handleGetConversation)
	mux.HandleFunc("PATCH /api/conversations/{id}", s.handlePatchConversation)
	mux.HandleFunc("DELETE /api/conversations/{id}", s.handleDeleteConversation)
	mux.HandleFunc("GET /api/conversations/{id}/messages", s.handleListMessages)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.handlePostMessage)
	mux.HandleFunc("POST /api/conversations/{id}/generate", s.handleGenerate)
	mux.HandleFunc("POST /api/conversations/{id}/regenerate", s.handleRegenerate)
	mux.HandleFunc("POST /api/conversations/{id}/stop", s.handleStop)
	mux.HandleFunc("GET /api/conversations/{id}/export", s.handleExport)

	// Settings, models, events.
	mux.HandleFunc("GET /api/settings", s.handleGetSettings)
	mux.HandleFunc("PATCH /api/settings", s.handlePatchSettings)
	mux.HandleFunc("GET /api/models", s.handleModels)
	mux.HandleFunc("GET /api/events", s.handleEvents)

	return s.middleware(mux)
}

// ---- middleware chain -----------------------------------------------------

func (s *Server) middleware(next http.Handler) http.Handler {
	return withRequestID(withSecurity(withAccessLog(s.log, withRecover(s.log, next))))
}

func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-Id", id)
		ctx := observability.WithRequestID(r.Context(), id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline'; "+ // inline styles are used for message theming
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func withAccessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip noisy heartbeat/access log for asset + events traffic at debug.
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rw, r)
		if strings.HasPrefix(r.URL.Path, "/static/") || strings.HasSuffix(r.URL.Path, "/events") {
			log.Debug("request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Int64("ms", time.Since(start).Milliseconds()),
				slog.String("request_id", observability.RequestID(r.Context())),
			)
			return
		}
		log.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rw.status),
			slog.Int64("ms", time.Since(start).Milliseconds()),
			slog.String("request_id", observability.RequestID(r.Context())),
		)
	})
}

func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("panic recovered",
					slog.Any("panic", rec),
					slog.String("request_id", observability.RequestID(r.Context())),
				)
				respondError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (r *statusRecorder) WriteHeader(code int) {
	if !r.wrote {
		r.status = code
		r.wrote = true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	r.wrote = true
	return r.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for SSE handlers wrapped by statusRecorder.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ---- JSON helpers ---------------------------------------------------------

func (s *Server) json(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		s.log.Error("encode response", slog.String("err", err.Error()))
	}
}

func respondError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

// decodeJSON reads a bounded JSON body into dst. A nil/empty body is allowed
// (leaves dst untouched).
func (s *Server) decodeJSON(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	body := http.MaxBytesReader(nil, r.Body, s.maxBodyBytes)
	defer body.Close()
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	err := dec.Decode(dst)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func (s *Server) pathID(r *http.Request) string {
	id := r.PathValue("id")
	if !isValidUUID(id) {
		return ""
	}
	return id
}

func isValidUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	for i, c := range s {
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return false
			}
		default:
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
				return false
			}
		}
	}
	return true
}

// ---- Hub: broadcast events to /api/events subscribers ---------------------

// Hub fans out server-sent events to connected clients.
type Hub struct {
	mu   sync.Mutex
	subs map[chan string]struct{}
}

// NewHub creates a Hub.
func NewHub() *Hub { return &Hub{subs: map[chan string]struct{}{}} }

// Subscribe registers a subscriber channel.
func (h *Hub) Subscribe() chan string {
	ch := make(chan string, 16)
	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

// Unsubscribe removes a subscriber.
func (h *Hub) Unsubscribe(ch chan string) {
	h.mu.Lock()
	delete(h.subs, ch)
	close(ch)
	h.mu.Unlock()
}

// Publish broadcasts an event string to all subscribers (non-blocking).
func (h *Hub) Publish(event string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

// ---- SSE helpers ----------------------------------------------------------

// sseWriter writes Server-Sent Events and flushes each frame.
type sseWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
	log     *slog.Logger
}

func newSSE(w http.ResponseWriter) (*sseWriter, bool) {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &sseWriter{w: w, flusher: f, log: nil}, true
}

func (sw *sseWriter) event(name string, data any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if _, err := sw.w.Write([]byte("event: " + name + "\ndata: " + string(b) + "\n\n")); err != nil {
		return err
	}
	sw.flusher.Flush()
	return nil
}

func (sw *sseWriter) comment(text string) {
	_, _ = sw.w.Write([]byte(": " + text + "\n\n"))
	sw.flusher.Flush()
}

// ctxOrBackground guards against nil contexts in helpers.
func ctxOrBackground(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
