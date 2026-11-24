// Package mimir is a minimal client for the Mimir inference service, which
// exposes an OpenAI-compatible API (the deployed backend is llama.cpp's
// llama-server). It implements chat completions with streaming, model
// discovery, and a small request-compatibility layer so that backend-specific
// knobs (notably Qwen3 "thinking") are applied correctly.
package mimir

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Role constants for chat messages.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Backend identifies the upstream serving implementation, which controls how
// request payloads are shaped.
type Backend string

const (
	BackendAuto     Backend = "auto"
	BackendLLaMACpp Backend = "llamacpp"
	BackendOpenAI   Backend = "openai"
)

// Message is a single chat turn.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest describes a chat-completion request.
type ChatRequest struct {
	Model       string
	Messages    []Message
	Temperature float64
	MaxTokens   int
	Thinking    bool
	Stream      bool
}

// Usage reports token counts.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Delta is an incremental token update.
type Delta struct {
	Content   string
	Reasoning string
}

// Result is the aggregate outcome of a completion.
type Result struct {
	Content        string
	Reasoning      string
	FinishReason   string
	Model          string
	Usage          Usage
	HasUsage       bool
	TruncatedByLen bool // finish_reason == "length"
}

// APIError is returned for non-2xx upstream responses.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("mimir: upstream returned %d: %s", e.StatusCode, e.Message)
}

// ErrContextOverflow signals the prompt exceeded the model context window.
var ErrContextOverflow = errors.New("mimir: context overflow")

// ErrCanceled signals the generation was canceled by the caller.
var ErrCanceled = errors.New("mimir: canceled")

// ModelInfo describes a model reported by /v1/models.
type ModelInfo struct {
	ID         string
	OwnedBy    string
	ContextLen int
}

// Client talks to the Mimir OpenAI-compatible API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	backend Backend

	// discovered model metadata (populated via Discover)
	model   string
	modelID string
	ctxLen  int
}

// NewClient builds a client. baseURL should include the version prefix, e.g.
// http://inference:8080/v1. timeout bounds non-streaming calls and connection
// establishment; streaming calls are bounded by the caller's context.
func NewClient(baseURL, apiKey string, timeout time.Duration, backend Backend) *Client {
	transport := &http.Transport{
		MaxIdleConns:        16,
		MaxIdleConnsPerHost: 8,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false,
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		backend: backend,
		http: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

// SetHTTPClient allows tests to inject a custom client/transport.
func (c *Client) SetHTTPClient(h *http.Client) { c.http = h }

// Model returns the effective model identifier.
func (c *Client) Model() string {
	if c.model != "" {
		return c.model
	}
	return c.modelID
}

// Backend resolves the effective backend (auto-detection result if applicable).
func (c *Client) Backend() Backend { return c.backend }

func (c *Client) setAuth(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// ---- model discovery ------------------------------------------------------

type modelsResponse struct {
	Data []struct {
		ID       string `json:"id"`
		OwnedBy  string `json:"owned_by"`
		Meta     struct {
			Nctx int `json:"n_ctx"`
		} `json:"meta"`
	} `json:"data"`
	// Ollama-style alternative shape.
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Discover queries /v1/models and returns the first model plus context length
// and backend hint. It also caches the model id for later requests.
func (c *Client) Discover(ctx context.Context) (ModelInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return ModelInfo{}, err
	}
	c.setAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("mimir: discover: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ModelInfo{}, readAPIError(resp)
	}
	var mr modelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return ModelInfo{}, fmt.Errorf("mimir: discover decode: %w", err)
	}
	var info ModelInfo
	if len(mr.Data) > 0 {
		info.ID = mr.Data[0].ID
		info.OwnedBy = mr.Data[0].OwnedBy
		info.ContextLen = mr.Data[0].Meta.Nctx
	} else if len(mr.Models) > 0 {
		info.ID = mr.Models[0].Name
	}
	if info.ID == "" {
		return ModelInfo{}, fmt.Errorf("mimir: no models reported")
	}
	c.modelID = info.ID
	if c.model == "" {
		c.model = info.ID
	}
	if info.ContextLen > 0 {
		c.ctxLen = info.ContextLen
	}
	// Auto-detect backend from owned_by / model naming.
	if c.backend == BackendAuto || c.backend == "" {
		c.backend = detectBackend(info.OwnedBy, info.ID)
	}
	return info, nil
}

// SetModel explicitly pins the model identifier (used when MIMIR_MODEL is set).
func (c *Client) SetModel(id string) { c.model = id }

// SetContextLen overrides the known model context length.
func (c *Client) SetContextLen(n int) {
	if n > 0 {
		c.ctxLen = n
	}
}

// ContextLen returns the discovered model context length (0 if unknown).
func (c *Client) ContextLen() int { return c.ctxLen }

func detectBackend(ownedBy, id string) Backend {
	s := strings.ToLower(ownedBy + " " + id)
	switch {
	case strings.Contains(s, "llamacpp"), strings.Contains(s, "llama.cpp"),
		strings.Contains(s, "llama-server"), strings.Contains(s, ".gguf"):
		return BackendLLaMACpp
	default:
		return BackendOpenAI
	}
}

// ---- health ---------------------------------------------------------------

// Health performs a cheap connectivity check and returns round-trip latency.
func (c *Client) Health(ctx context.Context) (latencyMs int64, model string, err error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return 0, "", err
	}
	c.setAuth(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return time.Since(start).Milliseconds(), "", readAPIError(resp)
	}
	io.Copy(io.Discard, resp.Body)
	return time.Since(start).Milliseconds(), c.Model(), nil
}

// ---- request building (compatibility layer) -------------------------------

// buildBody renders the JSON request body for the configured backend. The key
// compatibility decision is how "thinking" is disabled: llama.cpp and vLLM
// honour a top-level chat_template_kwargs.enable_thinking flag (the OpenAI SDK
//'s extra_body flattens to this over the wire), which we send for llama.cpp.
func (c *Client) buildBody(req ChatRequest) ([]byte, error) {
	model := req.Model
	if model == "" {
		model = c.Model()
	}
	body := map[string]any{
		"model":       model,
		"messages":    req.Messages,
		"temperature": req.Temperature,
	}
	if req.MaxTokens > 0 {
		body["max_tokens"] = req.MaxTokens
	}
	if req.Stream {
		body["stream"] = true
		body["stream_options"] = map[string]any{"include_usage": true}
	}

	// Thinking control for Qwen3-family models.
	applyThinking(&body, c.backend, req.Thinking)

	return json.Marshal(body)
}

func applyThinking(body *map[string]any, backend Backend, thinking bool) {
	switch backend {
	case BackendLLaMACpp:
		if !thinking {
			(*body)["chat_template_kwargs"] = map[string]any{"enable_thinking": false}
		}
		// When thinking is enabled we omit the flag entirely and let the model
		// reason normally (reasoning is streamed in delta.reasoning_content).
	case BackendOpenAI:
		// Reasoning models use reasoning_effort; harmless for others is not
		// guaranteed, so only set it when explicitly enabled/disabled is safe.
		if !thinking {
			(*body)["reasoning_effort"] = "none"
		}
	}
}

// ---- chat completions -----------------------------------------------------

// ChatCompletion performs a non-streaming completion. Used for health probes
// and title fallbacks.
func (c *Client) ChatCompletion(ctx context.Context, req ChatRequest) (*Result, error) {
	req.Stream = false
	body, err := c.buildBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, body, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, classifyTransportErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, readAPIError(resp)
	}
	var out struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Role             string `json:"role"`
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage *Usage `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("mimir: decode completion: %w", err)
	}
	res := &Result{Model: out.Model}
	if len(out.Choices) > 0 {
		ch := out.Choices[0]
		res.Content = ch.Message.Content
		res.Reasoning = ch.Message.ReasoningContent
		res.FinishReason = ch.FinishReason
		res.TruncatedByLen = ch.FinishReason == "length"
	}
	if out.Usage != nil {
		res.Usage = *out.Usage
		res.HasUsage = true
	}
	return res, nil
}

// ChatCompletionStream performs a streaming completion, invoking onDelta for
// each incremental chunk. Returning a non-nil error from onDelta aborts the
// stream. Cancellation of ctx aborts the HTTP request, which closes the
// connection to Mimir and propagates cancellation upstream.
func (c *Client) ChatCompletionStream(ctx context.Context, req ChatRequest, onDelta func(Delta) error) (*Result, error) {
	req.Stream = true
	body, err := c.buildBody(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := c.newRequest(ctx, body, true)
	if err != nil {
		return nil, err
	}

	// Streaming uses a client without the global timeout so long generations
	// are bounded only by the caller's context.
	streamClient := *c.http
	streamClient.Timeout = 0
	resp, err := streamClient.Do(httpReq)
	if err != nil {
		return nil, classifyTransportErr(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		return nil, readAPIError(resp)
	}

	res := &Result{}
	parsed, err := parseSSE(ctx, resp.Body, func(ev sseEvent) error {
		if ev.Done {
			return nil
		}
		var chunk streamChunk
		if err := json.Unmarshal(ev.Data, &chunk); err != nil {
			return fmt.Errorf("mimir: bad chunk: %w", err)
		}
		if chunk.Model != "" {
			res.Model = chunk.Model
		}
		if chunk.Usage != nil {
			res.Usage = *chunk.Usage
			res.HasUsage = true
		}
		for _, ch := range chunk.Choices {
			if ch.FinishReason != "" {
				res.FinishReason = ch.FinishReason
				res.TruncatedByLen = ch.FinishReason == "length"
			}
			d := Delta{Content: ch.Delta.Content, Reasoning: ch.Delta.ReasoningContent}
			if d.Content != "" {
				res.Content += d.Content
			}
			if d.Reasoning != "" {
				res.Reasoning += d.Reasoning
			}
			if (d.Content != "" || d.Reasoning != "") && onDelta != nil {
				if err := onDelta(d); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if parsed {
		return res, nil
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

type streamChunk struct {
	Model   string  `json:"model"`
	Choices []struct {
		Delta struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
			Role             string `json:"role"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
}

func (c *Client) newRequest(ctx context.Context, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	c.setAuth(req)
	return req, nil
}

func readAPIError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	msg := strings.TrimSpace(string(body))
	var parsed struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.Error.Message != "" {
		msg = parsed.Error.Message
	}
	if msg == "" {
		msg = http.StatusText(resp.StatusCode)
	}
	if isOverflowMessage(msg) || resp.StatusCode == http.StatusRequestEntityTooLarge {
		return fmt.Errorf("%w: %s", ErrContextOverflow, msg)
	}
	return &APIError{StatusCode: resp.StatusCode, Message: msg}
}

func classifyTransportErr(err error) error {
	if errors.Is(err, context.Canceled) {
		return ErrCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("mimir: timeout: %w", err)
	}
	return fmt.Errorf("mimir: request: %w", err)
}

func isOverflowMessage(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "context size has been exceeded") ||
		strings.Contains(m, "exceed") && strings.Contains(m, "context") ||
		strings.Contains(m, "too many tokens")
}
