package core

import (
	"testing"

	"gud/internal/profile"

	"github.com/spf13/cobra"
)

// TestHookModeToleratesUncachedProfile verifies that hook mode never blocks a
// git commit because a configured profile is not cached. The tolerant
// constructor must succeed and keep the profile name (whose content degrades
// to "" via resolveProfileContent), while the strict constructor used by
// normal mode still rejects the missing profile.
func TestHookModeToleratesUncachedProfile(t *testing.T) {
	orig := profileManager
	t.Cleanup(func() { profileManager = orig })
	profileManager = profile.NewManagerWithDir(t.TempDir())

	t.Setenv("GUD_CONFIG_PATH", t.TempDir()+"/nonexistent.json")

	cmd := &cobra.Command{}
	addPersistentFlags(cmd)
	// Parse flags so cobra merges the persistent flags into cmd.Flags(),
	// mirroring how configFromCmd observes them during a real execution.
	if err := cmd.ParseFlags([]string{"--profile", "nonexistent-slug-12345"}); err != nil {
		t.Fatalf("parse flags: %v", err)
	}

	// Tolerant constructor (hook mode) must succeed despite the uncached profile.
	app, err := NewAppContextTolerant(cmd)
	if err != nil {
		t.Fatalf("NewAppContextTolerant with uncached profile: %v", err)
	}
	if app == nil {
		t.Fatal("NewAppContextTolerant returned nil app")
	}
	if got := app.Config().Profile; got != "nonexistent-slug-12345" {
		t.Errorf("Profile = %q, want %q", got, "nonexistent-slug-12345")
	}
	// Content resolution degrades gracefully, matching generate.go behaviour.
	if got := resolveProfileContent(string(app.Config().Profile)); got != "" {
		t.Errorf("resolveProfileContent() = %q, want ''", got)
	}

	// Strict constructor (normal mode) must still reject the missing profile.
	if _, err := NewAppContext(cmd); err == nil {
		t.Error("NewAppContext with uncached profile should return an error")
	}
}

func TestHasMeaningfulContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty string", text: "", want: false},
		{name: "only blank lines", text: "\n\n  \n", want: false},
		{name: "only git comments", text: "# Please enter commit message\n# Lines starting with # are comments", want: false},
		{name: "single real line", text: "feat: add login", want: true},
		{name: "real line with leading text", text: "  feat: add login\n", want: true},
		{name: "comments then content", text: "# Please enter commit message\nfeat: add login\n# more comments", want: true},
		{name: "content then comments", text: "fix: resolve crash\n# Co-authored-by: someone@example.com", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasMeaningfulContent(tt.text)
			if got != tt.want {
				t.Errorf("hasMeaningfulContent(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
