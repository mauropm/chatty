package chat

import (
	"crypto/rand"
	"encoding/hex"
	"unicode/utf8"

	"chatty/internal/mimir"
)

// newToken returns a short random identifier to tie a release function to the
// specific generation it registered.
func newToken() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// estimateTokens gives a conservative upper-bound token estimate for a message
// list. Accuracy is not critical (Mimir clamps the real window), but over-
// estimating slightly is safe: it trims history earlier, protecting the small
// context window of the deployed model.
func estimateTokens(msgs []mimir.Message) int {
	const perMessageOverhead = 6
	total := 0
	for _, m := range msgs {
		runes := utf8.RuneCountInString(m.Content)
		// ~1 token per 2 runes is conservative for Latin text and appropriate
		// for CJK on the Qwen tokenizer.
		total += runes/2 + perMessageOverhead
	}
	return total
}

// computeBudget reconciles the desired completion size and prompt budget with
// the model's real context window, so a request never provably overflows.
// It returns (maxTokens, promptTokenBudget).
func computeBudget(modelCtx, wantMaxTokens, wantPromptBudget int) (int, int) {
	const reserve = 128        // safety headroom for chat-template tokens
	const minPromptFloor = 256 // never starve the prompt of room
	if modelCtx <= 0 {
		modelCtx = 4096
	}
	if wantMaxTokens <= 0 {
		wantMaxTokens = 512
	}
	available := modelCtx - reserve
	if available < 64 {
		available = 64
	}

	// Clamp completion so at least minPromptFloor remains for the prompt.
	maxTokens := wantMaxTokens
	if ceiling := available - minPromptFloor; maxTokens > ceiling {
		maxTokens = ceiling
	}
	if maxTokens < 1 {
		maxTokens = 1
	}

	promptBudget := wantPromptBudget
	if room := available - maxTokens; promptBudget > room {
		promptBudget = room
	}
	if promptBudget < 64 {
		promptBudget = 64
	}
	return maxTokens, promptBudget
}
