package chat

import (
	"testing"

	"chatty/internal/mimir"
)

func TestComputeBudgetTightModelContext(t *testing.T) {
	// Deployed Qwen model: n_ctx 2048. A large default max_tokens must be
	// clamped so prompt+completion can never exceed the window.
	maxTokens, prompt := computeBudget(2048, 4096, 1400)
	if maxTokens+prompt > 2048-128+5 {
		t.Errorf("budget overflows window: max=%d prompt=%d", maxTokens, prompt)
	}
	if maxTokens < 1 {
		t.Errorf("maxTokens too small: %d", maxTokens)
	}
	if prompt < 64 {
		t.Errorf("prompt budget too small: %d", prompt)
	}
	if maxTokens != 1664 { // 2048-128-256
		t.Errorf("maxTokens = %d, want 1664", maxTokens)
	}
}

func TestComputeBudgetComfortable(t *testing.T) {
	maxTokens, prompt := computeBudget(32768, 1024, 1400)
	if maxTokens != 1024 {
		t.Errorf("maxTokens = %d, want 1024 (unchanged)", maxTokens)
	}
	if prompt != 1400 {
		t.Errorf("prompt = %d, want 1400 (unchanged)", prompt)
	}
}

func TestComputeBudgetDefaultsWhenUnknown(t *testing.T) {
	maxTokens, prompt := computeBudget(0, 0, 0)
	if maxTokens < 1 || prompt < 64 {
		t.Errorf("bad defaults: max=%d prompt=%d", maxTokens, prompt)
	}
}

func TestEstimateTokensGrowsWithContent(t *testing.T) {
	small := estimateTokens([]mimir.Message{{Role: "user", Content: "hi"}})
	large := estimateTokens([]mimir.Message{{Role: "user", Content: "word " + str(500)}})
	if large <= small {
		t.Errorf("expected larger estimate, got small=%d large=%d", small, large)
	}
	if small <= 0 {
		t.Errorf("estimate must be positive")
	}
}

func str(n int) string {
	b := make([]byte, 0, n)
	for i := 0; i < n; i++ {
		b = append(b, 'a')
	}
	return string(b)
}
