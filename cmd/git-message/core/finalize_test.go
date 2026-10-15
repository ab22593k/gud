package core

import (
	"bytes"
	"log/slog"
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

// TestFinalizeMessageAssistedBySuppression pins FR-037: an explicit opt-out
// removes only the Assisted-by trailer. Issue trailers and body content are
// unchanged, while the default configuration keeps the footer.
func TestFinalizeMessageAssistedBySuppression(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		suppressed   bool
		wantAssisted bool
	}{
		{name: "explicit opt-out omits footer", suppressed: true, wantAssisted: false},
		{name: "default keeps footer", suppressed: false, wantAssisted: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body := "Explain why the change is necessary without repeating the diff."
			cfg := config.Config{
				WrapLine:     60,
				Issues:       []int{7},
				NoAssistedBy: config.Ptr(tt.suppressed),
			}

			got, err := finalizeMessage(t.Context(), "fix: resolve leak\n\n"+body, cfg, "test-model")
			if err != nil {
				t.Fatalf("finalizeMessage: %v", err)
			}

			if !strings.Contains(got, "Fixes: #7") {
				t.Errorf("finalized message lost the Fixes trailer:\n%q", got)
			}

			if hasAssisted := strings.Contains(got, "Assisted-by: test-model"); hasAssisted != tt.wantAssisted {
				t.Errorf("Assisted-by present = %v, want %v:\n%q", hasAssisted, tt.wantAssisted, got)
			}

			joined := strings.Join(strings.Fields(got), " ")
			for word := range strings.FieldsSeq(body) {
				if !strings.Contains(joined, word) {
					t.Errorf("finalized message lost body word %q:\n%q", word, got)
				}
			}
		})
	}
}

// TestFinalizeMessageSuppressedRecordsServingModel pins the second half of
// FR-037: suppression removes the trailer from the message, not the serving
// model from diagnostics.
func TestFinalizeMessageSuppressedRecordsServingModel(t *testing.T) {
	var buf bytes.Buffer

	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	cfg := config.Config{WrapLine: 60, NoAssistedBy: config.Ptr(true)}

	if _, err := finalizeMessage(t.Context(), "fix: resolve leak\n\nExplain why.", cfg, "test-model"); err != nil {
		t.Fatalf("finalizeMessage: %v", err)
	}

	diagnostics := buf.String()
	if !strings.Contains(diagnostics, "assisted-by trailer suppressed") {
		t.Errorf("diagnostics did not record suppression:\n%q", diagnostics)
	}

	if !strings.Contains(diagnostics, "test-model") {
		t.Errorf("diagnostics did not record the serving model:\n%q", diagnostics)
	}
}
