package mimir

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// recorded captures what the client sent so tests can assert on the payload.
type recorded struct {
	Path string
	Body map[string]any
}

// newMockServer returns a fake OpenAI-compatible backend and a recorder.
func newMockServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *recorded) {
	t.Helper()
	rec := &recorded{}
	base := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.Path = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &rec.Body)
		handler(w, r)
	})
	srv := httptest.NewServer(base)
	t.Cleanup(srv.Close)
	return srv, rec
}

func sseBody(events ...string) string {
	var sb strings.Builder
	for _, e := range events {
		sb.WriteString("data: " + e + "\n\n")
	}
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

func TestDiscoverFindsModelAndBackend(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"data":[{"id":"/models/Qwen3-1.7B-Q4_K_M.gguf","owned_by":"llamacpp","meta":{"n_ctx":2048}}]}`)
			return
		}
		http.NotFound(w, r)
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendAuto)
	info, err := c.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if info.ID != "/models/Qwen3-1.7B-Q4_K_M.gguf" {
		t.Errorf("id = %q", info.ID)
	}
	if info.ContextLen != 2048 {
		t.Errorf("ctx = %d, want 2048", info.ContextLen)
	}
	if c.Backend() != BackendLLaMACpp {
		t.Errorf("backend = %q, want llamacpp", c.Backend())
	}
	if c.Model() != info.ID {
		t.Errorf("cached model = %q", c.Model())
	}
}

func TestThinkingDisabledSendsChatTemplateKwargs(t *testing.T) {
	srv, rec := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	c.SetModel("qwen")
	res, err := c.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}}, Temperature: 0.7, MaxTokens: 128, Thinking: false,
	})
	if err != nil {
		t.Fatalf("ChatCompletion: %v", err)
	}
	if res.Content != "ok" || res.Usage.CompletionTokens != 2 {
		t.Errorf("result = %+v", res)
	}
	ktw, ok := rec.Body["chat_template_kwargs"].(map[string]any)
	if !ok {
		t.Fatalf("missing chat_template_kwargs: %v", rec.Body)
	}
	if et, ok := ktw["enable_thinking"].(bool); !ok || et {
		t.Errorf("enable_thinking = %v, want false", ktw["enable_thinking"])
	}
	if rec.Body["max_tokens"].(float64) != 128 {
		t.Errorf("max_tokens not forwarded")
	}
}

func TestThinkingEnabledOmitsKwargs(t *testing.T) {
	srv, rec := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"thinking done"},"finish_reason":"stop"}]}`)
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	if _, err := c.ChatCompletion(context.Background(), ChatRequest{
		Messages: []Message{{Role: RoleUser, Content: "hi"}}, Thinking: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, present := rec.Body["chat_template_kwargs"]; present {
		t.Errorf("chat_template_kwargs should be absent when thinking enabled")
	}
}

func TestStreamParsesContentUsageAndDone(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"delta":{"role":"assistant","content":null}}],"model":"m"}`,
			`{"choices":[{"delta":{"content":"Hel"}}]}`,
			`{"choices":[{"delta":{"content":"lo"}}]}`,
			`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`,
		}
		fmt.Fprint(w, sseBody(chunks...))
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	var streamed strings.Builder
	res, err := c.ChatCompletionStream(context.Background(),
		ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}},
		func(d Delta) error { streamed.WriteString(d.Content); return nil })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if streamed.String() != "Hello" {
		t.Errorf("streamed = %q, want Hello", streamed.String())
	}
	if res.Content != "Hello" {
		t.Errorf("res.Content = %q", res.Content)
	}
	if res.FinishReason != "stop" {
		t.Errorf("finish = %q", res.FinishReason)
	}
	if !res.HasUsage || res.Usage.PromptTokens != 10 || res.Usage.CompletionTokens != 4 {
		t.Errorf("usage = %+v (has=%v)", res.Usage, res.HasUsage)
	}
}

func TestStreamReasoningSeparated(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(
			`{"choices":[{"delta":{"reasoning_content":"think "}}]}`,
			`{"choices":[{"delta":{"content":"answer"}}]}`,
		))
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	res, err := c.ChatCompletionStream(context.Background(),
		ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}, Thinking: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Reasoning != "think " {
		t.Errorf("reasoning = %q", res.Reasoning)
	}
	if res.Content != "answer" {
		t.Errorf("content = %q", res.Content)
	}
}

func TestStreamCancellation(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\n\n")
		if fl != nil {
			fl.Flush()
		}
		<-r.Context().Done() // block until client disconnects
	})
	c := NewClient(srv.URL, "", 30*time.Second, BackendLLaMACpp)
	ctx, cancel := context.WithCancel(context.Background())
	_, err := c.ChatCompletionStream(ctx, ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}},
		func(d Delta) error { cancel(); return nil })
	if err != ErrCanceled {
		t.Errorf("expected ErrCanceled, got %v", err)
	}
}

func TestStreamAbortsWhenOnDeltaErrors(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(
			`{"choices":[{"delta":{"content":"a"}}]}`,
			`{"choices":[{"delta":{"content":"b"}}]}`,
		))
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	myErr := fmt.Errorf("boom")
	_, err := c.ChatCompletionStream(context.Background(),
		ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}},
		func(d Delta) error { return myErr })
	if err != myErr {
		t.Errorf("expected onDelta error to propagate, got %v", err)
	}
}

func TestUpstreamErrorBecomesAPIError(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":{"message":"gpu on fire"}}`)
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	_, err := c.ChatCompletion(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	var ae *APIError
	if !errorsAs(err, &ae) {
		t.Fatalf("expected APIError, got %T %v", err, err)
	}
	if ae.StatusCode != 500 || !strings.Contains(ae.Message, "gpu on fire") {
		t.Errorf("apiError = %+v", ae)
	}
}

func TestContextOverflowDetection(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error":{"message":"context size has been exceeded, but was requested by client"}}`)
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	_, err := c.ChatCompletion(context.Background(), ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}})
	if !errorsIs(err, ErrContextOverflow) {
		t.Errorf("expected ErrContextOverflow, got %v", err)
	}
}

func TestOnDeltaEarlyAbortKeepsPartial(t *testing.T) {
	srv, _ := newMockServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseBody(
			`{"choices":[{"delta":{"content":"par"}}]}`,
			`{"choices":[{"delta":{"content":"tial"}}]}`,
			`{"choices":[{"delta":{"content":"-more"}}]}`,
		))
	})
	c := NewClient(srv.URL, "", 5*time.Second, BackendLLaMACpp)
	count := 0
	res, _ := c.ChatCompletionStream(context.Background(),
		ChatRequest{Messages: []Message{{Role: RoleUser, Content: "x"}}},
		func(d Delta) error {
			count++
			if count >= 2 {
				return fmt.Errorf("stop")
			}
			return nil
		})
	if res.Content != "partial" {
		t.Errorf("partial content = %q, want 'partial'", res.Content)
	}
}

// small helpers to avoid importing errors in every test
func errorsIs(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func errorsAs(err error, target any) bool {
	switch tp := target.(type) {
	case **APIError:
		for err != nil {
			if ae, ok := err.(*APIError); ok {
				*tp = ae
				return true
			}
			u, ok := err.(interface{ Unwrap() error })
			if !ok {
				return false
			}
			err = u.Unwrap()
		}
	}
	return false
}
