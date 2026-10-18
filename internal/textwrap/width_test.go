package textwrap

import (
	"strings"
	"testing"
)

func TestColumns(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "abc", 3},
		{"cjk double width", "漢", 2},
		{"emoji double width", "🦀", 2},
		{"mixed", "a漢b", 4},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := Columns(tc.in); got != tc.want {
				t.Errorf("Columns(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()

	t.Run("long line wraps within limit", func(t *testing.T) {
		t.Parallel()

		longLine := "This is a long commit message paragraph that exceeds forty characters."
		wrapped := Wrap(longLine, 40)

		for line := range strings.Lines(wrapped) {
			if w := Columns(strings.TrimSuffix(line, "\n")); w > 40 {
				t.Errorf("line is %d cells, want <= 40: %q", w, line)
			}
		}

		if !strings.Contains(wrapped, "\n") {
			t.Errorf("long line was not wrapped:\n%q", wrapped)
		}
	})

	t.Run("within limit unchanged", func(t *testing.T) {
		t.Parallel()

		in := "fix: resolve leak\n\nFree buffers on error."

		if got := Wrap(in, 72); got != in {
			t.Errorf("Wrap changed text already within limit:\n%q", got)
		}
	})

	t.Run("non-positive limit unchanged", func(t *testing.T) {
		t.Parallel()

		in := "a very long line that would otherwise be wrapped somewhere in the middle of the text"

		if got := Wrap(in, 0); got != in {
			t.Errorf("Wrap with limit 0 changed text:\n%q", got)
		}

		if got := Wrap(in, -1); got != in {
			t.Errorf("Wrap with negative limit changed text:\n%q", got)
		}
	})

	t.Run("indentation preserved", func(t *testing.T) {
		t.Parallel()

		in := "  indented continuation line that runs past the forty column mark here"
		wrapped := Wrap(in, 40)

		for line := range strings.Lines(wrapped) {
			trimmed := strings.TrimSuffix(line, "\n")
			if !strings.HasPrefix(trimmed, "  ") {
				t.Errorf("wrapped line lost indentation: %q", trimmed)
			}
		}
	})

	t.Run("never truncates content", func(t *testing.T) {
		t.Parallel()

		// A single over-long word has no wrap point: words stay atomic, so
		// the line exceeds the limit rather than losing content.
		in := strings.Repeat("x", 100)
		wrapped := Wrap(in, 40)

		if strings.ReplaceAll(wrapped, "\n", "") != in {
			t.Errorf("Wrap lost content:\n%q", wrapped)
		}
	})
}

// TestWrapUnbreakableToken pins the one exception to "every presented line fits
// the limit" (contracts/verification.md §2.1): a single token longer than the
// limit is emitted intact and over-width, it is the only over-width line, and
// the token is never split. Every line that carries a space still folds within
// the limit, so the exception is narrow and disclosed, not a quiet hole.
func TestWrapUnbreakableToken(t *testing.T) {
	t.Parallel()

	const limit = 40

	// One unbreakable token (a URL is the canonical case) well past the limit.
	token := "https://example.test/" + strings.Repeat("segment", 20)

	cases := []struct {
		name string
		in   string
	}{
		{name: "single over-long token", in: token},
		{name: "over-long token among words", in: "See " + token + " for the resolved defect"},
		{name: "over-long token mid-sentence", in: "fold these words " + token + " and these too"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := Wrap(tc.in, limit)
			lines := strings.Split(got, "\n")

			// (iii) the token survives whole, never split mid-token.
			if !strings.Contains(got, token) {
				t.Errorf("Wrap split the unbreakable token:\n%q", got)
			}

			// (i) the only over-width line is the raw token itself.
			for _, line := range lines {
				if Columns(line) > limit && line != token {
					t.Errorf("over-width line is not the intact token: %q", line)
				}
			}

			// (ii) every line carrying a space still folds within the limit.
			for _, line := range lines {
				if strings.Contains(line, " ") && Columns(line) > limit {
					t.Errorf("spaced line exceeded the limit: %q", line)
				}
			}
		})
	}
}

// TestWrapMeasuresCells guards CJK/emoji wrapping: limits are terminal
// cells, not bytes. Eight 3-CJK-char words are 79 bytes but 55 cells, so a
// 72-col limit must keep them on one line; byte counting wrapped them.
func TestWrapMeasuresCells(t *testing.T) {
	t.Parallel()

	cjk := strings.TrimSpace(strings.Repeat("漢漢漢 ", 8))

	if got := Wrap(cjk, 72); len(strings.Split(got, "\n")) != 1 {
		t.Errorf("CJK line of 55 cells wrapped at 72 cols:\n%q", got)
	}

	// Emoji words behave the same way: eight 3-crab words are 103 bytes but
	// 55 cells, so they stay on one line; twelve are 83 cells and must wrap
	// with every line within limit. Words stay atomic (long URLs are never
	// split mid-token), matching the pre-existing wrapping contract.
	emojiFit := strings.TrimSpace(strings.Repeat("🦀🦀🦀 ", 8))
	if got := Wrap(emojiFit, 72); len(strings.Split(got, "\n")) != 1 {
		t.Errorf("55-cell emoji line wrapped at 72 cols:\n%q", got)
	}

	emojiWide := strings.TrimSpace(strings.Repeat("🦀🦀🦀 ", 12))
	lines := strings.Split(Wrap(emojiWide, 72), "\n")

	if len(lines) < 2 {
		t.Fatalf("83-cell emoji line was not wrapped")
	}

	for _, line := range lines {
		if w := Columns(line); w > 72 {
			t.Errorf("wrapped line is %d cells, want <= 72: %q", w, line)
		}
	}
}
