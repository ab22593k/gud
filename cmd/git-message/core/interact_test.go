package core

import (
	"os"
	"path/filepath"
	"testing"

	"gud/internal/git"
	"gud/internal/mem"
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

func TestToFileChanges(t *testing.T) {
	t.Parallel()

	const (
		mainFile  = "main.go"
		changeMod = "modified"
	)

	tests := []struct {
		name  string
		units []git.CodeUnit
		want  []mem.FileChange
	}{
		{
			name:  "empty input returns nil",
			units: nil,
			want:  nil,
		},
		{
			name: "single unit",
			units: []git.CodeUnit{
				{FilePath: mainFile, ChangeType: changeMod},
			},
			want: []mem.FileChange{
				{Path: mainFile, ChangeType: changeMod},
			},
		},
		{
			name: "duplicate file paths are deduplicated",
			units: []git.CodeUnit{
				{FilePath: mainFile, Name: "Run", ChangeType: changeMod},
				{FilePath: mainFile, Name: "Stop", ChangeType: changeMod},
			},
			want: []mem.FileChange{
				{Path: mainFile, ChangeType: changeMod},
			},
		},
		{
			name: "multiple unique files preserved",
			units: []git.CodeUnit{
				{FilePath: "a.go", ChangeType: "added"},
				{FilePath: "b.go", ChangeType: changeMod},
			},
			want: []mem.FileChange{
				{Path: "a.go", ChangeType: "added"},
				{Path: "b.go", ChangeType: changeMod},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toFileChanges(tt.units)
			if len(got) != len(tt.want) {
				t.Fatalf("toFileChanges() returned %d items, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Path != tt.want[i].Path || got[i].ChangeType != tt.want[i].ChangeType {
					t.Errorf("toFileChanges()[%d] = {Path:%q, ChangeType:%q}, want {Path:%q, ChangeType:%q}",
						i, got[i].Path, got[i].ChangeType, tt.want[i].Path, tt.want[i].ChangeType)
				}
			}
		})
	}
}

func TestToCodeUnitRefs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		units []git.CodeUnit
		want  []mem.CodeUnitRef
	}{
		{
			name:  "empty input returns nil",
			units: nil,
			want:  nil,
		},
		{
			name: "single function mapped",
			units: []git.CodeUnit{
				{Name: "Render", Kind: "function", ChangeType: "deleted", FilePath: "main.go"},
			},
			want: []mem.CodeUnitRef{
				{Name: "Render", Kind: "function", ChangeType: "deleted", FilePath: "main.go"},
			},
		},
		{
			name: "all fields preserved including method receiver",
			units: []git.CodeUnit{
				{Name: "(*App).Bootstrap", Kind: "method", ChangeType: "removed", FilePath: "app.go"},
			},
			want: []mem.CodeUnitRef{
				{Name: "(*App).Bootstrap", Kind: "method", ChangeType: "removed", FilePath: "app.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := toCodeUnitRefs(tt.units)
			if len(got) != len(tt.want) {
				t.Fatalf("toCodeUnitRefs() returned %d items, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("toCodeUnitRefs()[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}
