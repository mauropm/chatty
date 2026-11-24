// Package tests holds integration tests that require a real PostgreSQL
// instance. They are skipped unless CHATTY_TEST_DATABASE=1 is set, so the
// ordinary `go test ./...` run stays hermetic.
package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"chatty/internal/chat"
	"chatty/internal/config"
	"chatty/internal/conversations"
	"chatty/internal/database"
	"chatty/internal/messages"
	"chatty/internal/mimir"
	"chatty/internal/settings"
	"chatty/internal/statistics"
)

// setupDB connects to PostgreSQL using CHATTY_* env and runs migrations. It
// uses a disposable schema so repeated runs are isolated.
func setupDB(t *testing.T) (*database.DB, *config.Config) {
	t.Helper()
	if os.Getenv("CHATTY_TEST_DATABASE") != "1" {
		t.Skip("set CHATTY_TEST_DATABASE=1 to run integration tests against PostgreSQL")
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	cfg, err := config.Parse(env(map[string]string{
		"DATABASE_HOST":     getenv("CHATTY_TEST_DB_HOST", "chatty-db"),
		"DATABASE_PORT":     getenv("CHATTY_TEST_DB_PORT", "5432"),
		"DATABASE_NAME":     getenv("CHATTY_TEST_DB_NAME", "chatty"),
		"DATABASE_USER":     getenv("CHATTY_TEST_DB_USER", "chatty"),
		"DATABASE_PASSWORD": getenv("CHATTY_TEST_DB_PASSWORD", ""),
		"DATABASE_SSLMODE":  "disable",
		"MIMIR_BASE_URL":    "http://inference:8080/v1",
		"CHAT_MAX_TOKENS":   "512",
		"CHAT_MAX_CONTEXT_TOKENS": "900",
	}))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	db, err := database.Connect(ctx, cfg.Database, log)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, cfg
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// mockOpenAI is a fake OpenAI-compatible upstream for engine tests.
type mockOpenAI struct {
	srv          *httptest.Server
	calls        int64
	lastThinking *bool
	lastCtx      map[string]any
}

func newMockOpenAI(t *testing.T, reply string, streamChunks int) *mockOpenAI {
	m := &mockOpenAI{}
	m.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&m.calls, 1)
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		m.lastCtx = body
		if kt, ok := body["chat_template_kwargs"].(map[string]any); ok {
			if et, ok := kt["enable_thinking"].(bool); ok {
				v := et
				m.lastThinking = &v
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		// stream reply in N chunks
		step := 1
		if streamChunks > 1 {
			step = max(1, len(reply)/streamChunks)
		}
		for i := 0; i < len(reply); i += step {
			end := min(i+step, len(reply))
			data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": reply[i:end]}}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
			if fl != nil {
				fl.Flush()
			}
		}
		stop, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{}, "finish_reason": "stop"}}})
		usage, _ := json.Marshal(map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 42, "completion_tokens": len(reply) / 4, "total_tokens": 42 + len(reply)/4}})
		fmt.Fprintf(w, "data: %s\n\ndata: %s\n\ndata: [DONE]\n\n", stop, usage)
	}))
	t.Cleanup(m.srv.Close)
	return m
}

func min(a, b int) int { if a < b { return a }; return b }
func max(a, b int) int { if a > b { return a }; return b }

func newEngine(db *database.DB, cfg *config.Config, client *mimir.Client) *chat.Engine {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	conv := conversations.NewRepo(db.Pool)
	msgs := messages.NewRepo(db.Pool)
	stats := statistics.NewService(db.Pool)
	set := settings.NewService(db.Pool, cfg)
	return chat.NewEngine(conv, msgs, client, stats, set, cfg, log)
}

func TestIntegration_ConversationLifecycle(t *testing.T) {
	db, _ := setupDB(t)
	ctx := context.Background()
	repo := conversations.NewRepo(db.Pool)

	c, err := repo.Create(ctx, "  ") // empty → default title
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.Title != "New conversation" {
		t.Errorf("default title = %q", c.Title)
	}
	if err := repo.Rename(ctx, c.ID, "My rename"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	got, err := repo.Get(ctx, c.ID)
	if err != nil || got.Title != "My rename" {
		t.Fatalf("get after rename: %v %+v", err, got)
	}
	// archive + exclude from default list
	if err := repo.SetArchived(ctx, c.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	list, _ := repo.List(ctx, conversations.ListOptions{})
	for _, x := range list {
		if x.ID == c.ID {
			t.Error("archived conversation should be excluded from default list")
		}
	}
	withArchived, _ := repo.List(ctx, conversations.ListOptions{IncludeArchived: true})
	if !containsConv(withArchived, c.ID) {
		t.Error("archived conversation should appear when include_archived")
	}
	// search by title
	found, _ := repo.Search(ctx, "rename", true, 10)
	if !containsConv(found, c.ID) {
		t.Error("search by title should find conversation")
	}
	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.Get(ctx, c.ID); err != conversations.ErrNotFound {
		t.Errorf("expected not found, got %v", err)
	}
}

func containsConv(cs []*conversations.Conversation, id string) bool {
	for _, c := range cs {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestIntegration_MessageOrderingAndContext(t *testing.T) {
	db, _ := setupDB(t)
	ctx := context.Background()
	conv := conversations.NewRepo(db.Pool)
	msgs := messages.NewRepo(db.Pool)
	c, _ := conv.Create(ctx, "order test")

	for _, txt := range []string{"one", "two", "three"} {
		if _, err := msgs.Append(ctx, c.ID, messages.RoleUser, txt, messages.StatusComplete, ""); err != nil {
			t.Fatal(err)
		}
	}
	// A failed assistant must never appear in context.
	failed, _ := msgs.AppendAssistantPending(ctx, c.ID, "m")
	msgs.Finalize(ctx, failed.ID, messages.FinalizeOpts{Content: "bad", Status: messages.StatusFailed})

	list, err := msgs.List(ctx, c.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 4 {
		t.Fatalf("want 4 messages, got %d", len(list))
	}
	for i, m := range list[:3] {
		if m.Content != []string{"one", "two", "three"}[i] {
			t.Errorf("ordering wrong at %d: %q", i, m.Content)
		}
	}
	ctxMsgs, err := msgs.ListForContext(ctx, c.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ctxMsgs {
		if m.Status != messages.StatusComplete {
			t.Errorf("context should exclude non-complete: %q %s", m.Content, m.Status)
		}
		if m.Role == messages.RoleAssistant && m.Content == "bad" {
			t.Error("failed assistant leaked into context")
		}
	}
}

func TestIntegration_GenerateStreamsAndPersists(t *testing.T) {
	db, cfg := setupDB(t)
	mock := newMockOpenAI(t, "The quick brown fox.", 4)
	client := mimir.NewClient(mock.srv.URL+"/v1", "", 20*time.Second, mimir.BackendLLaMACpp)
	client.SetModel("test-model")
	client.SetContextLen(2048)
	engine := newEngine(db, cfg, client)

	conv := conversations.NewRepo(db.Pool)
	msgs := messages.NewRepo(db.Pool)
	stats := statistics.NewService(db.Pool)
	c, _ := conv.Create(context.Background(), "")

	var events []chat.Event
	_, err := engine.Generate(context.Background(), c.ID, chat.GenerateOptions{Content: "Tell me a sentence"}, func(e chat.Event) error {
		events = append(events, e)
		return nil
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	// thinking disabled → chat_template_kwargs.enable_thinking == false
	if mock.lastThinking == nil || *mock.lastThinking {
		t.Errorf("expected enable_thinking=false, got %v", mock.lastThinking)
	}
	stored, _ := msgs.List(context.Background(), c.ID, 0)
	if len(stored) != 2 {
		t.Fatalf("want user+assistant, got %d", len(stored))
	}
	asst := stored[1]
	if asst.Role != messages.RoleAssistant || asst.Status != messages.StatusComplete {
		t.Errorf("assistant not complete: %+v", asst)
	}
	if asst.Content != "The quick brown fox." {
		t.Errorf("persisted content = %q", asst.Content)
	}
	if asst.CompletionTokens == 0 || asst.PromptTokens != 42 {
		t.Errorf("usage not persisted: prompt=%d completion=%d", asst.PromptTokens, asst.CompletionTokens)
	}
	// title auto-generated from first user message
	cc, _ := conv.Get(context.Background(), c.ID)
	if cc.Title == "" || cc.Title == "New conversation" {
		t.Errorf("title not auto-generated: %q", cc.Title)
	}
	// events include meta + delta + done
	kinds := map[string]int{}
	for _, e := range events {
		kinds[e.Type]++
	}
	if kinds["meta"] != 1 || kinds["delta"] == 0 || kinds["done"] != 1 {
		t.Errorf("unexpected event kinds: %v", kinds)
	}
	// usage event recorded
	ov, _ := stats.Overview(context.Background())
	if ov.TotalRequests < 1 || ov.SuccessfulRequests < 1 {
		t.Errorf("usage events not recorded: %+v", ov)
	}
}

func TestIntegration_InterruptedGenerationPersistsPartial(t *testing.T) {
	db, cfg := setupDB(t)
	// A mock that streams some tokens then hangs until the client cancels.
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		fmt.Fprintf(w, "data: %s\n\n", `{"choices":[{"delta":{"content":"Partial answer begin"}}]}`)
		if fl != nil {
			fl.Flush()
		}
		close(started)
		<-r.Context().Done() // hold the stream open until client disconnects
	}))
	defer srv.Close()

	client := mimir.NewClient(srv.URL+"/v1", "", 60*time.Second, mimir.BackendLLaMACpp)
	client.SetModel("test-model")
	client.SetContextLen(2048)
	engine := newEngine(db, cfg, client)
	conv := conversations.NewRepo(db.Pool)
	msgs := messages.NewRepo(db.Pool)
	c, _ := conv.Create(context.Background(), "")

	reqCtx, cancel := context.WithCancel(context.Background())
	go func() { <-started; time.Sleep(50 * time.Millisecond); cancel() }()
	_, _ = engine.Generate(reqCtx, c.ID, chat.GenerateOptions{Content: "Say something long"}, func(chat.Event) error { return nil })

	list, _ := msgs.List(context.Background(), c.ID, 0)
	var asst *messages.Message
	for _, m := range list {
		if m.Role == messages.RoleAssistant {
			asst = m
		}
	}
	if asst == nil {
		t.Fatal("assistant message missing")
	}
	if asst.Status != messages.StatusStopped {
		t.Errorf("status = %s, want stopped", asst.Status)
	}
	if !strings.Contains(asst.Content, "Partial answer") {
		t.Errorf("partial content not persisted: %q", asst.Content)
	}
}

func TestIntegration_RegenerateReplacesLastAssistant(t *testing.T) {
	db, cfg := setupDB(t)
	mock := newMockOpenAI(t, "Regenerated reply", 3)
	client := mimir.NewClient(mock.srv.URL+"/v1", "", 20*time.Second, mimir.BackendLLaMACpp)
	client.SetModel("test-model"); client.SetContextLen(2048)
	engine := newEngine(db, cfg, client)
	conv := conversations.NewRepo(db.Pool)
	msgs := messages.NewRepo(db.Pool)
	c, _ := conv.Create(context.Background(), "")

	engine.Generate(context.Background(), c.ID, chat.GenerateOptions{Content: "hello there"}, func(chat.Event) error { return nil })
	before, _ := msgs.Count(context.Background(), c.ID)
	engine.Generate(context.Background(), c.ID, chat.GenerateOptions{Regenerate: true}, func(chat.Event) error { return nil })
	after, _ := msgs.Count(context.Background(), c.ID)
	if before != after {
		t.Errorf("message count changed on regenerate: %d → %d", before, after)
	}
	last, _ := msgs.LastAssistant(context.Background(), c.ID)
	if last.Content != "Regenerated reply" {
		t.Errorf("regenerate did not replace content: %q", last.Content)
	}
}

func TestIntegration_ConcurrentGenerationRejected(t *testing.T) {
	db, cfg := setupDB(t)
	// Slow mock that stays open.
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if fl, ok := w.(http.Flusher); ok {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			fl.Flush()
		}
		<-gate
	}))
	defer func() { close(gate); srv.Close() }()

	client := mimir.NewClient(srv.URL+"/v1", "", 60*time.Second, mimir.BackendLLaMACpp)
	client.SetModel("test-model"); client.SetContextLen(2048)
	engine := newEngine(db, cfg, client)
	conv := conversations.NewRepo(db.Pool)
	c, _ := conv.Create(context.Background(), "")

	firstDone := make(chan error, 1)
	go func() {
		_, err := engine.Generate(context.Background(), c.ID, chat.GenerateOptions{Content: "long thing"}, func(chat.Event) error { return nil })
		firstDone <- err
	}()
	// wait until the engine reports active, then attempt a second.
	deadline := time.Now().Add(3 * time.Second)
	for !engine.IsGenerating(c.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	_, err := engine.Generate(context.Background(), c.ID, chat.GenerateOptions{Content: "second"}, func(chat.Event) error { return nil })
	if err != chat.ErrBusy {
		t.Errorf("expected ErrBusy, got %v", err)
	}
	engine.Stop(c.ID)
	<-firstDone
}

// silence unused import if pgxpool not otherwise referenced
var _ = pgxpool.Pool{}
