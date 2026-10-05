package core

import (
	"os"
	"path/filepath"
	"testing"
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
