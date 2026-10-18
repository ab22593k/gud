package core

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/request"
	"gud/internal/textwrap"
)

func TestEditMessage(t *testing.T) {
	const msgHello = "hello world"

	t.Run("editor modifies content", func(t *testing.T) {
		editor := writeEditorScript(t, "#!/bin/sh\ncat > \"$1\" << 'EOF'\nmodified content\nEOF\n")
		t.Setenv("EDITOR", editor)

		got, err := editMessage("original content")
		if err != nil {
			t.Fatalf("editMessage() error = %v", err)
		}

		if got != "modified content" {
			t.Errorf("editMessage() = %q, want %q", got, "modified content")
		}
	})

	t.Run("editor leaves content unchanged", func(t *testing.T) {
		editor := writeEditorScript(t, "#!/bin/sh\ntrue\n")
		t.Setenv("EDITOR", editor)

		got, err := editMessage(msgHello)
		if err != nil {
			t.Fatalf("editMessage() error = %v", err)
		}

		if got != msgHello {
			t.Errorf("editMessage() = %q, want %q", got, msgHello)
		}
	})

	t.Run("editor failure returns error", func(t *testing.T) {
		editor := writeEditorScript(t, "#!/bin/sh\nexit 1\n")
		t.Setenv("EDITOR", editor)

		_, err := editMessage("test")
		if err == nil {
			t.Fatal("editMessage() expected error for editor failure")
		}
	})

	t.Run("content is trimmed", func(t *testing.T) {
		editor := writeEditorScript(t, "#!/bin/sh\nprintf '  hello world  \\n' > \"$1\"\n")
		t.Setenv("EDITOR", editor)

		got, err := editMessage("anything")
		if err != nil {
			t.Fatalf("editMessage() error = %v", err)
		}

		if got != "hello world" {
			t.Errorf("editMessage() = %q, want %q", got, "hello world")
		}
	})
}

// writeEditorScript writes a shell script to a temp file and returns its path.
// The script is made executable so it can be used as $EDITOR.
func writeEditorScript(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "editor.sh")

	//nolint:gosec // editor script must be executable to run as $EDITOR
	if err := os.WriteFile(path, []byte(content), 0700); err != nil {
		t.Fatalf("write editor script: %v", err)
	}

	return path
}

// TestPresentFindings pins the FR-018 interactive notice contract: a clean
// result (nil or empty findings) prints nothing, while surviving findings
// print exactly one line naming the failing checks. The presenter cannot
// fail — notices inform, they never error or change the exit status.
func TestPresentFindings(t *testing.T) {
	t.Parallel()

	t.Run("clean prints nothing", func(t *testing.T) {
		t.Parallel()

		for _, findings := range [][]request.Finding{nil, {}} {
			var out bytes.Buffer
			presentFindings(&out, findings)

			if out.String() != "" {
				t.Errorf("presentFindings(%v) printed %q, want nothing", findings, out.String())
			}
		}
	})

	t.Run("surviving findings name their checks", func(t *testing.T) {
		t.Parallel()

		findings := []request.Finding{
			{Check: "empty-body", Detail: "message has no body after the subject line"},
			{Check: "subject-length", Detail: "subject is 90 cols, limit is 72"},
		}

		var out bytes.Buffer
		presentFindings(&out, findings)

		got := out.String()
		if !strings.Contains(got, "empty-body") || !strings.Contains(got, "subject-length") {
			t.Errorf("notice = %q, want one line naming both failing checks", got)
		}

		if strings.Count(strings.TrimSuffix(got, "\n"), "\n") != 0 {
			t.Errorf("notice = %q, want exactly one line", got)
		}
	})

	t.Run("subject-length notice describes the draft, not the presented subject", func(t *testing.T) {
		t.Parallel()

		const wrapLine = 72

		// The verifier reports the advisory finding against the pre-finalization
		// draft, whose subject is a single over-limit line...
		draft := strings.TrimSpace(strings.Repeat("alpha ", 20)) + "\n\nExplain the change."
		findings := request.VerifyMessage(draft, "", wrapLine)

		if len(findings) != 1 || findings[0].Check != "subject-length" {
			t.Fatalf("VerifyMessage(draft) = %v, want one subject-length finding", findings)
		}

		// ...while finalization brings the presented subject within the limit.
		finalized, err := finalizeMessage(t.Context(), draft, config.Config{WrapLine: wrapLine}, "")
		if err != nil {
			t.Fatalf("finalizeMessage: %v", err)
		}

		subject, _, _ := strings.Cut(finalized, "\n")
		if w := textwrap.Columns(subject); w > wrapLine {
			t.Errorf("presented subject is %d cells, want <= %d: %q", w, wrapLine, subject)
		}

		var out bytes.Buffer
		presentFindings(&out, findings)

		notice := out.String()
		if !strings.Contains(notice, "subject-length") {
			t.Errorf("notice = %q, want the failing check named", notice)
		}

		if strings.Contains(strings.ToLower(notice), "too long") {
			t.Errorf("notice = %q, must not claim the presented subject is too long", notice)
		}
	})
}
