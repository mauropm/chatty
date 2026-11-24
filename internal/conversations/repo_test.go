package conversations

import (
	"strings"
	"testing"
)

func TestGenerateTitle(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"short", "How do I center a div in CSS?", "How do I center a div in CSS?"},
		{"markdown stripped", "```python\nprint('hi')\n``` explain", "python print('hi') explain"},
		{"collapse whitespace", "hello    world\n\there", "hello world here"},
		{"empty", "   ", "New conversation"},
		{"only symbols", "```", "New conversation"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := GenerateTitle(tc.input)
			if got != tc.want {
				t.Errorf("GenerateTitle(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestGenerateTitleTruncatesOnWordBoundary(t *testing.T) {
	long := strings.Repeat("word ", 40)
	got := GenerateTitle(long)
	if len([]rune(got)) > 62 { // 60 + ellipsis
		t.Errorf("title too long: %d runes", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("expected ellipsis suffix, got %q", got)
	}
	if strings.HasSuffix(strings.TrimSuffix(got, "…"), " ") {
		t.Errorf("trailing space before ellipsis: %q", got)
	}
}

func TestGenerateTitleNoWordBoundaryInWindow(t *testing.T) {
	long := strings.Repeat("x", 100)
	got := GenerateTitle(long)
	if len([]rune(got)) != 61 { // 60 + ellipsis
		t.Errorf("len = %d, want 61", len([]rune(got)))
	}
}
