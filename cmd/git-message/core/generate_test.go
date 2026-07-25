package core

import (
	"testing"

	"gud/internal/profile"
)

func TestResolveProfileContent_EmptyProfile(t *testing.T) {
	t.Parallel()

	if got := resolveProfileContent(""); got != "" {
		t.Errorf("resolveProfileContent('') = %q, want ''", got)
	}
}

func TestResolveProfileContent_NotFound(t *testing.T) {
	orig := profileManager
	t.Cleanup(func() { profileManager = orig })

	profileManager = profile.NewManagerWithDir(t.TempDir())
	if got := resolveProfileContent("nonexistent"); got != "" {
		t.Errorf("resolveProfileContent('nonexistent') = %q, want ''", got)
	}
}

func TestResolveProfileContent_Found(t *testing.T) {
	orig := profileManager
	t.Cleanup(func() { profileManager = orig })

	tmpDir := t.TempDir()
	m := profile.NewManagerWithDir(tmpDir)
	if err := m.Save("test-agent", profile.Profile{
		Content: "You are a test agent.",
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	profileManager = m

	got := resolveProfileContent("test-agent")
	if got != "You are a test agent." {
		t.Errorf("resolveProfileContent() = %q, want 'You are a test agent.'", got)
	}
}

func TestAppendDeletedContext(t *testing.T) {
	t.Parallel()

	const diff = "--- a/file.txt\n+++ b/file.txt\n@@ -1 +1 @@\n-old\n+new"

	tests := []struct {
		name    string
		diff    string
		deleted string
		want    string
	}{
		{
			name:    "no deleted files returns diff unchanged",
			diff:    diff,
			deleted: "",
			want:    diff,
		},
		{
			name:    "whitespace-only deleted returns diff unchanged",
			diff:    diff,
			deleted: "  \n\t\n  ",
			want:    diff,
		},
		{
			name:    "single deleted file appended",
			diff:    "diff --git a/old.go b/new.go",
			deleted: "old.go\n",
			want:    "diff --git a/old.go b/new.go\n\nDeleted files:\nold.go\n",
		},
		{
			name:    "multiple deleted files each on own line",
			diff:    "--- a/a.go\n+++ b/b.go",
			deleted: "a.go\nb.go\n",
			want:    "--- a/a.go\n+++ b/b.go\n\nDeleted files:\na.go\nb.go\n",
		},
		{
			name:    "filenames with whitespace are trimmed",
			diff:    "diff:",
			deleted: "  file.go  \n\tcmd/main.go\n",
			want:    "diff:\n\nDeleted files:\nfile.go\ncmd/main.go\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := appendDeletedContext(tt.diff, tt.deleted)
			if got != tt.want {
				t.Errorf("appendDeletedContext():\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}
