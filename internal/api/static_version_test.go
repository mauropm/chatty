package api

import (
	"bytes"
	"regexp"
	"strings"
	"testing"

	"chatty/web"
)

// TestVersionedIndex guards the asset cache-busting rewrite: every /static/
// asset URL must gain exactly one ?v=<hex> token, the original embedded bytes
// must not be mutated, and all script/link tags must survive the rewrite.
func TestVersionedIndex(t *testing.T) {
	orig := append([]byte(nil), web.Index()...)
	out := versionedIndex()

	if bytes.Equal(out, orig) {
		t.Fatal("index was not versioned")
	}
	if !bytes.Equal(orig, web.Index()) {
		t.Fatal("versionedIndex mutated the embedded index bytes")
	}
	html := string(out)

	tokens := regexp.MustCompile(`\?v=[a-f0-9]+`)
	doubles := regexp.MustCompile(`\?v=[a-f0-9]+\?v=`)
	if doubles.FindString(html) != "" {
		t.Fatalf("duplicated version query found: %s", doubles.FindString(html))
	}
	if n := len(tokens.FindAllString(html, -1)); n < 3 {
		t.Fatalf("expected >=3 versioned URLs, got %d", n)
	}

	for _, want := range []string{"app.css", "app.js", "boot.js", "waiting-messages.js"} {
		if !strings.Contains(html, want+"?v=") {
			t.Fatalf("%s missing or not versioned", want)
		}
	}
	for _, want := range []string{"<script", "stylesheet", "</html>"} {
		if !strings.Contains(html, want) {
			t.Fatalf("rewrite destroyed document structure: %q missing", want)
		}
	}
	if n := strings.Count(html, "<script"); n != strings.Count(string(orig), "<script") {
		t.Fatalf("script tag count changed: %d vs %d", n, strings.Count(string(orig), "<script"))
	}
}
