package core

import (
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/textwrap"
)

// TestFinalizeMessage pins the width-conformance contract (FR-013, SC-003):
// trailers are appended first and survive normalization, and afterwards every
// line — subject, body, and trailers — fits WrapLine display cells on every
// presentation path. Long lines are wrapped, never truncated.
func TestFinalizeMessage(t *testing.T) {
	t.Parallel()

	cfg := config.Config{WrapLine: 60, Issues: []int{7}}
	longBody := "This body line runs well past the sixty column mark and must be folded " +
		"onto continuation lines rather than cut, keeping every word it carries."

	got, err := finalizeMessage(t.Context(), "fix: resolve leak\n\n"+longBody, cfg, "test-model")
	if err != nil {
		t.Fatalf("finalizeMessage: %v", err)
	}

	for line := range strings.SplitSeq(got, "\n") {
		if w := textwrap.Columns(line); w > 60 {
			t.Errorf("finalized line is %d cells, want <= 60: %q", w, line)
		}
	}

	if !strings.Contains(got, "Fixes: #7") {
		t.Errorf("finalized message lost the Fixes trailer:\n%q", got)
	}

	if !strings.Contains(got, "Assisted-by: test-model") {
		t.Errorf("finalized message lost the Assisted-by trailer:\n%q", got)
	}

	// Wrapping folds; it must never drop content.
	joined := strings.Join(strings.Fields(got), " ")
	for word := range strings.FieldsSeq(longBody) {
		if !strings.Contains(joined, word) {
			t.Errorf("finalized message lost body word %q:\n%q", word, got)
		}
	}
}

// TestFinalizeMessageGitAuthored pins FR-016/FR-022: a git-authored message
// (empty model identity) is still width-normalized but carries no
// Assisted-by trailer.
func TestFinalizeMessageGitAuthored(t *testing.T) {
	t.Parallel()

	cfg := config.Config{WrapLine: 60}

	got, err := finalizeMessage(t.Context(), "Merge branch 'topic'\n\nCombine prior work.", cfg, "")
	if err != nil {
		t.Fatalf("finalizeMessage: %v", err)
	}

	if strings.Contains(got, "Assisted-by") {
		t.Errorf("git-authored message gained an Assisted-by trailer:\n%q", got)
	}

	for line := range strings.SplitSeq(got, "\n") {
		if w := textwrap.Columns(line); w > 60 {
			t.Errorf("finalized line is %d cells, want <= 60: %q", w, line)
		}
	}
}

// TestFinalizeMessageSupportedBounds pins the supported WrapLine range: the
// clamp floor and ceiling both normalize without failing.
func TestFinalizeMessageSupportedBounds(t *testing.T) {
	t.Parallel()

	msg := "fix: resolve leak\n\n" + strings.Repeat("word ", 40)

	for _, wl := range []int{40, 200} {
		got, err := finalizeMessage(t.Context(), msg, config.Config{WrapLine: wl}, "m")
		if err != nil {
			t.Fatalf("finalizeMessage(wrapline=%d): %v", wl, err)
		}

		for line := range strings.SplitSeq(got, "\n") {
			if w := textwrap.Columns(line); w > wl {
				t.Errorf("wrapline=%d: line is %d cells: %q", wl, w, line)
			}
		}
	}
}
