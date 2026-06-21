package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// --- dispWidth ---

func TestDispWidth(t *testing.T) {
	cases := []struct {
		s    string
		want int
	}{
		{"abc", 3},
		{"가나", 4},  // 2 fullwidth = 4 columns
		{"가b다", 5}, // 2+1+2
		{"", 0},
		{"서울특별시", 10}, // 5 Hangul = 10 columns
	}
	for _, c := range cases {
		if got := dispWidth(c.s); got != c.want {
			t.Errorf("dispWidth(%q) = %d, want %d", c.s, got, c.want)
		}
	}
}

// --- truncate (display-width, rune-safe) ---

func TestTruncate_Hangul(t *testing.T) {
	out := truncate("가나다라마", 6)
	if !utf8.ValidString(out) {
		t.Fatalf("truncate produced invalid UTF-8: %q", out)
	}
	if dispWidth(out) > 6 {
		t.Errorf("truncate width %d exceeds 6: %q", dispWidth(out), out)
	}
	if !strings.HasSuffix(out, "..") {
		t.Errorf("expected ellipsis suffix, got %q", out)
	}
}

func TestTruncate_ASCIIRegression(t *testing.T) {
	if got := truncate("abcdefgh", 5); got != "abc.." {
		t.Errorf("truncate ASCII: got %q, want %q", got, "abc..")
	}
	if got := truncate("short", 10); got != "short" {
		t.Errorf("truncate no-op: got %q, want %q", got, "short")
	}
}

func TestTruncate_TinyWidth(t *testing.T) {
	// maxWidth too small for an ellipsis: must still produce valid UTF-8 within width.
	out := truncate("가나다", 2)
	if !utf8.ValidString(out) {
		t.Fatalf("invalid UTF-8: %q", out)
	}
	if dispWidth(out) > 2 {
		t.Errorf("width %d exceeds 2: %q", dispWidth(out), out)
	}
}

// --- table alignment: every rendered line shares the same display width ---

func TestRenderTable_HangulAlignment(t *testing.T) {
	data := []map[string]interface{}{
		{"name": "서울특별시", "code": "11"},
		{"name": "Busan", "code": "26"},
	}
	tmp := t.TempDir()
	out := filepath.Join(tmp, "out.txt")
	if err := Render(rawJSON(data), Options{Format: "table", OutFile: out}); err != nil {
		t.Fatalf("Render: %v", err)
	}
	content, _ := os.ReadFile(out)
	var widthSeen = -1
	for _, line := range strings.Split(strings.TrimRight(string(content), "\n"), "\n") {
		w := dispWidth(line)
		if widthSeen == -1 {
			widthSeen = w
		} else if w != widthSeen {
			t.Errorf("misaligned line (width %d != %d): %q", w, widthSeen, line)
		}
	}
}
