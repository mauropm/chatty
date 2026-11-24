// Package chat orchestrates the conversational workflow: it persists the user
// turn, builds a bounded model context, streams a completion from Mimir while
// forwarding deltas to the browser, persists the (possibly partial) assistant
// response, records usage, and supports stop / regenerate.
package chat

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"chatty/internal/config"
	"chatty/internal/conversations"
	"chatty/internal/messages"
	"chatty/internal/mimir"
	"chatty/internal/observability"
	"chatty/internal/settings"
	"chatty/internal/statistics"
)

// Sentinel errors surfaced to the API layer.
var (
	ErrConversationNotFound = errors.New("chat: conversation not found")
	ErrBusy                 = errors.New("chat: a generation is already in progress for this conversation")
	ErrNoPrompt             = errors.New("chat: nothing to generate from")
)

// Event types emitted to the browser over SSE.
const (
	EventMeta      = "meta"
	EventDelta     = "delta"
	EventReasoning = "reasoning"
	EventDone      = "done"
	EventError     = "error"
)

// Event is a single SSE notification forwarded to the client.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Emitter forwards an event to the client. Returning an error aborts the
// generation (e.g. the browser closed the connection).
type Emitter func(Event) error

// Engine coordinates chat operations.
type Engine struct {
	conv     *conversations.Repo
	msgs     *messages.Repo
	mimir    *mimir.Client
	stats    *statistics.Service
	settings *settings.Service
	cfg      *config.Config
	log      *slog.Logger

	mu     sync.Mutex
	active map[string]*generation // conversationID -> in-flight generation
}

type generation struct {
	cancel    context.CancelFunc
	token     string
	messageID string
	started   time.Time
}

// NewEngine wires the chat engine.
func NewEngine(
	conv *conversations.Repo,
	msgs *messages.Repo,
	mimirClient *mimir.Client,
	stats *statistics.Service,
	set *settings.Service,
	cfg *config.Config,
	log *slog.Logger,
) *Engine {
	return &Engine{
		conv:     conv,
		msgs:     msgs,
		mimir:    mimirClient,
		stats:    stats,
		settings: set,
		cfg:      cfg,
		log:      log,
		active:   map[string]*generation{},
	}
}

// GenerateOptions configures a generate request.
type GenerateOptions struct {
	// Content, when non-empty, is appended as a new user message first.
	Content string
	// Regenerate drops the last assistant message and regenerates a reply to
	// the current last user message.
	Regenerate bool
	// UserMessageID, when set, reuses an already-created user message instead
	// of appending a new one.
	UserMessageID string
}

// Generate runs the full workflow and streams events via emit. It returns the
// terminal assistant message (may be partial). The request ctx is used to
// cancel the upstream call on client disconnect; database finalization uses a
// detached context so a partial response is always persisted.
func (e *Engine) Generate(ctx context.Context, conversationID string, opt GenerateOptions, emit Emitter) (*messages.Message, error) {
	reqLog := observability.LoggerFrom(ctx, e.log).With(slog.String("conversation_id", conversationID))

	// Ensure the conversation exists before doing anything.
	conv, err := e.conv.Get(ctx, conversationID)
	if err != nil {
		if errors.Is(err, conversations.ErrNotFound) {
			return nil, ErrConversationNotFound
		}
		return nil, err
	}

	// Create a generation context derived from the request so that either a
	// client disconnect or an explicit Stop cancels the upstream Mimir call.
	genCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	release, err := e.register(conversationID, cancel)
	if err != nil {
		return nil, err
	}
	defer release()

	gen, err := e.settings.Get(ctx)
	if err != nil {
		reqLog.Warn("settings read failed, using defaults", slog.String("err", err.Error()))
		gen = e.settings.Defaults()
	}

	// Persist the new user turn (or reuse one).
	switch {
	case opt.Content != "":
		if _, err := e.msgs.Append(ctx, conversationID, messages.RoleUser, opt.Content, messages.StatusComplete, ""); err != nil {
			return nil, fmt.Errorf("chat: append user message: %w", err)
		}
		if conv.Title == "" || conv.Title == "New conversation" {
			if t := conversations.GenerateTitle(opt.Content); t != "New conversation" {
				_ = e.conv.Rename(ctx, conversationID, t)
			}
		}
	case opt.Regenerate:
		// Remove the assistant reply being replaced.
		if last, err := e.msgs.LastAssistant(ctx, conversationID); err == nil && last != nil {
			_ = e.msgs.DeleteMessage(ctx, last.ID)
		}
	case opt.UserMessageID != "":
		if _, err := e.msgs.Get(ctx, opt.UserMessageID); err != nil {
			return nil, err
		}
	}

	// Build the model context.
	ctxMsgs, model, maxTokens, err := e.buildContext(ctx, conversationID, gen)
	if err != nil {
		return nil, err
	}
	if !hasUserTurn(ctxMsgs) {
		return nil, ErrNoPrompt
	}

	// Create the assistant message up-front so an interruption can persist it.
	assistant, err := e.msgs.AppendAssistantPending(ctx, conversationID, model)
	if err != nil {
		return nil, fmt.Errorf("chat: append assistant: %w", err)
	}
	e.setActive(conversationID, assistant.ID)

	if err := emit(Event{Type: EventMeta, Data: map[string]any{
		"message_id":      assistant.ID,
		"conversation_id": conversationID,
		"model":           model,
		"created_at":      assistant.CreatedAt,
	}}); err != nil {
		e.finalize(conversationID, assistant.ID, "", messages.StatusStopped, model, nil, 0, "client disconnected before generation")
		return nil, err
	}

	start := time.Now()
	result, streamErr := e.mimir.ChatCompletionStream(genCtx, mimir.ChatRequest{
		Model:       model,
		Messages:    ctxMsgs,
		Temperature: gen.Temperature,
		MaxTokens:   maxTokens,
		Thinking:    gen.Thinking,
		Stream:      true,
	}, func(d mimir.Delta) error {
		if d.Reasoning != "" {
			if err := emit(Event{Type: EventReasoning, Data: map[string]any{"content": d.Reasoning}}); err != nil {
				return err
			}
		}
		if d.Content != "" {
			return emit(Event{Type: EventDelta, Data: map[string]any{"content": d.Content}})
		}
		return nil
	})
	latency := time.Since(start).Milliseconds()
	if result == nil {
		result = &mimir.Result{}
	}

	// Determine terminal status. Cancellation can arrive as our sentinel, a
	// context error surfaced from the body read, or a canceled generation
	// context (stop button / client disconnect).
	content := result.Content
	canceled := errors.Is(streamErr, mimir.ErrCanceled) ||
		errors.Is(streamErr, context.Canceled) ||
		errors.Is(ctx.Err(), context.Canceled) ||
		genCtx.Err() != nil
	var status, errMsg string
	switch {
	case canceled:
		status = messages.StatusStopped
		if content == "" {
			errMsg = "generation stopped"
		}
		reqLog.Info("generation stopped", slog.Int64("latency_ms", latency), slog.Int("chars", len(content)))
	case streamErr != nil:
		status = messages.StatusFailed
		if errors.Is(streamErr, mimir.ErrContextOverflow) {
			errMsg = mimir.ErrContextOverflow.Error()
		} else {
			errMsg = classifyError(streamErr)
		}
		reqLog.Error("generation failed", slog.String("err", streamErr.Error()))
	default:
		status = messages.StatusComplete
	}

	finalMsg, ferr := e.finalize(conversationID, assistant.ID, content, status, model, &result.Usage, latency, errMsg)

	// Persistence bookkeeping runs on a detached context so it survives a
	// client disconnect or canceled request context.
	dctx, dcancel := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
	defer dcancel()

	if rerr := e.stats.RecordUsage(dctx, statistics.UsageEvent{
		ConversationID:   conversationID,
		MessageID:        assistant.ID,
		Model:            model,
		PromptTokens:     usagePrompt(result),
		CompletionTokens: usageCompletion(result),
		LatencyMs:        latency,
		Success:          status == messages.StatusComplete,
		Metadata:         map[string]any{"finish_reason": result.FinishReason, "status": status},
	}); rerr != nil {
		reqLog.Warn("failed to record usage", slog.String("err", rerr.Error()))
	}

	// Hard failure (not a stop): surface a retryable error to the client.
	if streamErr != nil && !canceled {
		_ = emit(Event{Type: EventError, Data: map[string]any{
			"message":   errMsg,
			"retryable": true,
			"status":    status,
		}})
		if finalMsg != nil {
			return finalMsg, streamErr
		}
		return assistant, streamErr
	}

	if ferr != nil {
		reqLog.Error("failed to finalize assistant message", slog.String("err", ferr.Error()))
	}

	// Update conversation ordering + emit terminal event.
	_ = e.conv.Touch(dctx, conversationID)
	doneData := map[string]any{
		"message_id":        assistant.ID,
		"finish_reason":     result.FinishReason,
		"prompt_tokens":     usagePrompt(result),
		"completion_tokens": usageCompletion(result),
		"latency_ms":        latency,
		"status":            status,
	}
	if finalMsg != nil {
		doneData["content"] = finalMsg.Content
	}
	_ = emit(Event{Type: EventDone, Data: doneData})
	if finalMsg != nil {
		return finalMsg, nil
	}
	return assistant, nil
}

// Stop cancels an in-flight generation for a conversation, if any. It returns
// true if a generation was canceled.
func (e *Engine) Stop(conversationID string) bool {
	e.mu.Lock()
	g := e.active[conversationID]
	e.mu.Unlock()
	if g == nil {
		return false
	}
	g.cancel()
	return true
}

// IsGenerating reports whether a conversation has an in-flight generation.
func (e *Engine) IsGenerating(conversationID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.active[conversationID]
	return ok
}

// ActiveCount returns the number of in-flight generations.
func (e *Engine) ActiveCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.active)
}

// ---- internal helpers -----------------------------------------------------

// register adds an in-flight generation for a conversation and returns a
// release function. It fails with ErrBusy if one is already active.
func (e *Engine) register(conversationID string, cancel context.CancelFunc) (func(), error) {
	e.mu.Lock()
	if _, busy := e.active[conversationID]; busy {
		e.mu.Unlock()
		cancel()
		return nil, ErrBusy
	}
	token := newToken()
	e.active[conversationID] = &generation{cancel: cancel, token: token, started: time.Now()}
	e.mu.Unlock()

	released := false
	return func() {
		if released {
			return
		}
		released = true
		e.mu.Lock()
		// Only remove if the entry is still *our* generation.
		if g := e.active[conversationID]; g != nil && g.token == token {
			delete(e.active, conversationID)
		}
		e.mu.Unlock()
		cancel()
	}, nil
}

func (e *Engine) setActive(conversationID, messageID string) {
	e.mu.Lock()
	if g := e.active[conversationID]; g != nil {
		g.messageID = messageID
	}
	e.mu.Unlock()
}

func (e *Engine) finalize(conversationID, messageID, content, status, model string, usage *mimir.Usage, latency int64, errMsg string) (*messages.Message, error) {
	// Use a detached context so persistence completes even after the client or
	// upstream context is canceled.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 10*time.Second)
	defer cancel()
	opts := messages.FinalizeOpts{
		Content: content,
		Status:  status,
		Model:   model,
		LatencyMs: latency,
		Error:   errMsg,
	}
	if usage != nil {
		opts.PromptTokens = int64(usage.PromptTokens)
		opts.CompletionTokens = int64(usage.CompletionTokens)
	}
	msg, err := e.msgs.Finalize(ctx, messageID, opts)
	if err != nil {
		e.log.Error("finalize assistant message failed",
			slog.String("conversation_id", conversationID),
			slog.String("message_id", messageID),
			slog.String("err", err.Error()))
		return nil, err
	}
	return msg, nil
}

// buildContext assembles the model message list within the token budget and
// returns the effective model and clamped max_tokens.
func (e *Engine) buildContext(ctx context.Context, conversationID string, gen settings.Generation) ([]mimir.Message, string, int, error) {
	model := e.mimir.Model()
	if model == "" {
		model = e.cfg.Mimir.Model
	}

	history, err := e.msgs.ListForContext(ctx, conversationID, gen.MaxContextMessages)
	if err != nil {
		return nil, "", 0, err
	}

	out := make([]mimir.Message, 0, len(history)+1)
	if e.cfg.Chat.SystemPrompt != "" {
		out = append(out, mimir.Message{Role: mimir.RoleSystem, Content: e.cfg.Chat.SystemPrompt})
	}
	for _, m := range history {
		role := mimir.RoleUser
		if m.Role == messages.RoleAssistant {
			role = mimir.RoleAssistant
		}
		out = append(out, mimir.Message{Role: role, Content: m.Content})
	}

	// Determine context budget from the model context length.
	modelCtx := e.mimir.ContextLen()
	if modelCtx <= 0 {
		modelCtx = 4096
	}
	maxTokens, promptBudget := computeBudget(modelCtx, gen.MaxTokens, gen.MaxContextTokens)

	// Trim history from the oldest turn forward while over budget. The system
	// prompt (index 0 if present) and the final message are always kept.
	keepSystem := e.cfg.Chat.SystemPrompt != ""
	start := 0
	if keepSystem {
		start = 1
	}
	for estimateTokens(out) > promptBudget && len(out) > start+1 {
		// Remove the oldest non-system message (index start).
		out = append(out[:start], out[start+1:]...)
	}
	return out, model, maxTokens, nil
}

func usagePrompt(r *mimir.Result) int64 {
	if r == nil {
		return 0
	}
	return int64(r.Usage.PromptTokens)
}

func usageCompletion(r *mimir.Result) int64 {
	if r == nil {
		return 0
	}
	return int64(r.Usage.CompletionTokens)
}

func hasUserTurn(msgs []mimir.Message) bool {
	for _, m := range msgs {
		if m.Role == mimir.RoleUser {
			return true
		}
	}
	return false
}

func classifyError(err error) string {
	var ae *mimir.APIError
	if errors.As(err, &ae) {
		if ae.StatusCode >= 500 {
			return "The inference service is unavailable. Please try again."
		}
		return "The inference service rejected the request."
	}
	return "The inference service could not be reached."
}
