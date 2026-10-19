package core

import (
	"bytes"
	"context"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/git"
	"gud/internal/request"
	"gud/internal/textwrap"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

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

// hookTestEnv isolates the process from the developer's real environment:
// HOME keeps ~/.config/gud/hooks out of the real home directory, and
// GIT_CONFIG_GLOBAL redirects the global git config to a temp file.
func hookTestEnv(t *testing.T) context.Context {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("GUD_CONFIG_PATH", filepath.Join(t.TempDir(), "nonexistent.json"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "global-gitconfig"))

	return context.Background()
}

// TestHookInstallUninstallGlobalHooksPath verifies that a global install
// wires core.hooksPath to the gud hooks directory (idempotently) and a
// global uninstall clears both the hook file and the config value.
func TestHookInstallUninstallGlobalHooksPath(t *testing.T) {
	ctx := hookTestEnv(t)

	if err := runHookInstall(ctx, true); err != nil {
		t.Fatalf("runHookInstall(global): %v", err)
	}

	hookDir, err := git.GetHookDir(ctx, true)
	if err != nil {
		t.Fatalf("GetHookDir: %v", err)
	}

	hookPath := filepath.Join(hookDir, string(git.PrepareCommitMsg))
	if _, err := os.Stat(hookPath); err != nil {
		t.Errorf("hook file should exist at %s: %v", hookPath, err)
	}

	got, err := git.GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath: %v", err)
	}

	if got != hookDir {
		t.Errorf("core.hooksPath = %q, want %q", got, hookDir)
	}

	// Reinstall must stay idempotent.
	if err := runHookInstall(ctx, true); err != nil {
		t.Fatalf("reinstall: %v", err)
	}

	if err := runHookUninstall(ctx, true); err != nil {
		t.Fatalf("runHookUninstall(global): %v", err)
	}

	got, err = git.GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath after uninstall: %v", err)
	}

	if got != "" {
		t.Errorf("core.hooksPath = %q after uninstall, want empty", got)
	}

	if _, err := os.Stat(hookPath); !os.IsNotExist(err) {
		t.Errorf("hook file should be removed from %s", hookPath)
	}
}

// TestHookInstallKeepsForeignHooksPath verifies that install does not
// override a global core.hooksPath pointing somewhere else: the user's own
// hook configuration wins and gud's hook simply is not activated.
func TestHookInstallKeepsForeignHooksPath(t *testing.T) {
	ctx := hookTestEnv(t)

	const foreign = "/usr/local/other-hooks"
	if err := git.SetGlobalHooksPath(ctx, foreign); err != nil {
		t.Fatalf("SetGlobalHooksPath: %v", err)
	}

	if err := runHookInstall(ctx, true); err != nil {
		t.Fatalf("runHookInstall(global): %v", err)
	}

	got, err := git.GetGlobalHooksPath(ctx)
	if err != nil {
		t.Fatalf("GetGlobalHooksPath: %v", err)
	}

	if got != foreign {
		t.Errorf("core.hooksPath = %q, want foreign value %q preserved", got, foreign)
	}
}

// TestLogResidualFindings pins the FR-018/FR-028 unattended contract:
// surviving verification findings are surfaced through the diagnostics
// channel as a structured record carrying the check codes and the cause.
// A clean result logs nothing. The message-file write path is untouched.
func TestLogResidualFindings(t *testing.T) {
	t.Parallel()

	t.Run("clean logs nothing", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		log := slog.New(slog.NewTextHandler(&buf, nil))
		logResidualFindings(log, nil)
		logResidualFindings(log, []request.Finding{})

		if buf.String() != "" {
			t.Errorf("logResidualFindings(clean) logged %q, want nothing", buf.String())
		}
	})

	t.Run("surviving findings log codes and cause", func(t *testing.T) {
		t.Parallel()

		var buf bytes.Buffer

		log := slog.New(slog.NewTextHandler(&buf, nil))
		logResidualFindings(log, []request.Finding{
			{Check: "file-grounding", Detail: "mentions other.go, not present in diff"},
		})

		got := buf.String()
		if !strings.Contains(got, "file-grounding") {
			t.Errorf("record = %q, want the failing check code", got)
		}

		if !strings.Contains(got, "other.go") {
			t.Errorf("record = %q, want the finding detail", got)
		}
	})

	t.Run("subject-length record describes the draft, not the presented subject", func(t *testing.T) {
		t.Parallel()

		const wrapLine = 72

		// The advisory finding is raised against the draft subject...
		draft := strings.TrimSpace(strings.Repeat("alpha ", 20)) + "\n\nExplain the change."
		findings := request.VerifyMessage(draft, "", wrapLine)

		if len(findings) != 1 || findings[0].Check != "subject-length" {
			t.Fatalf("VerifyMessage(draft) = %v, want one subject-length finding", findings)
		}

		// ...while the presented subject is normalized within the limit.
		finalized, err := finalizeMessage(t.Context(), draft, config.Config{WrapLine: wrapLine}, "")
		if err != nil {
			t.Fatalf("finalizeMessage: %v", err)
		}

		subject, _, _ := strings.Cut(finalized, "\n")
		if w := textwrap.Columns(subject); w > wrapLine {
			t.Errorf("presented subject is %d cells, want <= %d: %q", w, wrapLine, subject)
		}

		var buf bytes.Buffer

		log := slog.New(slog.NewTextHandler(&buf, nil))
		logResidualFindings(log, findings)

		got := buf.String()
		if !strings.Contains(got, "subject-length") {
			t.Errorf("record = %q, want the failing check code", got)
		}

		if strings.Contains(strings.ToLower(got), "too long") {
			t.Errorf("record = %q, must not claim the presented subject is too long", got)
		}
	})
}

// TestGenerateAndWriteMsg_PreservesExistingMessage pins FR-026: a message file
// that already holds a user-supplied message is left untouched. The app carries
// no client, so a write would panic — returning cleanly proves generation was
// never attempted.
func TestGenerateAndWriteMsg_PreservesExistingMessage(t *testing.T) {
	t.Parallel()

	msgFile := filepath.Join(t.TempDir(), "COMMIT_EDITMSG")

	const existing = "fix: hand-written\n\nDo not overwrite this."

	if err := os.WriteFile(msgFile, []byte(existing), 0600); err != nil {
		t.Fatalf("write existing message: %v", err)
	}

	app := &AppContext{cfg: config.Config{WrapLine: 72}}

	if err := generateAndWriteMsg(t.Context(), app, "diff", msgFile); err != nil {
		t.Fatalf("generateAndWriteMsg: %v", err)
	}

	got, err := os.ReadFile(msgFile)
	if err != nil {
		t.Fatalf("read message file: %v", err)
	}

	if string(got) != existing {
		t.Errorf("existing message changed:\n got: %q\nwant: %q", got, existing)
	}
}

// TestGenerateAndWriteMsg_RefusesSymlink pins the file-safety guard on the
// unattended write: a message file that is a symbolic link is refused rather
// than followed, so generation cannot be redirected outside the intended path.
func TestGenerateAndWriteMsg_RefusesSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, nil, 0600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	app := &AppContext{cfg: config.Config{WrapLine: 72}}

	if err := generateAndWriteMsg(t.Context(), app, "diff", link); err == nil {
		t.Errorf("generateAndWriteMsg(symlink) = nil, want refusal")
	}
}

// symlinkSwapLLM replaces the message file with a symlink while the model call
// is in flight, reproducing the window between the Lstat guard and the final
// write.
type symlinkSwapLLM struct {
	msgFile string
	target  string
}

func (m *symlinkSwapLLM) Name() string { return "symlink-swap" }

func (m *symlinkSwapLLM) GenerateContent(
	_ context.Context, _ *model.LLMRequest, _ bool,
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if err := os.Remove(m.msgFile); err != nil {
			yield(nil, err)

			return
		}

		if err := os.Symlink(m.target, m.msgFile); err != nil {
			yield(nil, err)

			return
		}

		yield(&model.LLMResponse{Content: genai.NewContentFromText(
			"feat: keep the message path safe\n\nExplain the race window.", "model")}, nil)
	}
}

// TestGenerateAndWriteMsg_DoesNotFollowSymlinkSwappedDuringGeneration pins the
// TOCTOU fix: a symlink substituted after the Lstat guard but before the final
// write must not be followed.
func TestGenerateAndWriteMsg_DoesNotFollowSymlinkSwappedDuringGeneration(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	msgFile := filepath.Join(dir, "COMMIT_EDITMSG")
	if err := os.WriteFile(msgFile, []byte("# Please enter the commit message\n"), 0600); err != nil {
		t.Fatalf("write message file: %v", err)
	}

	target := filepath.Join(dir, "victim")

	const original = "original"

	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	probe := filepath.Join(dir, "symlink-probe")
	if err := os.Symlink(target, probe); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := os.Remove(probe); err != nil {
		t.Fatalf("remove symlink probe: %v", err)
	}

	suppress := true

	app := &AppContext{
		cfg:           config.Config{WrapLine: 72, NoAssistedBy: &suppress},
		client:        request.NewClientWithGenerator(&symlinkSwapLLM{msgFile: msgFile, target: target}, "mock"),
		agentsFilesOK: true,
	}

	if err := generateAndWriteMsg(t.Context(), app, "diff", msgFile); err != nil {
		t.Fatalf("generateAndWriteMsg: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}

	if string(got) != original {
		t.Errorf("victim target was overwritten: got %q, want %q", got, original)
	}

	fi, err := os.Lstat(msgFile)
	if err != nil {
		t.Fatalf("lstat message file: %v", err)
	}

	if !fi.Mode().IsRegular() {
		t.Errorf("message file mode = %v, want a regular file", fi.Mode())
	}

	msg, err := os.ReadFile(msgFile)
	if err != nil {
		t.Fatalf("read message file: %v", err)
	}

	if !strings.Contains(string(msg), "keep the message path safe") {
		t.Errorf("message file = %q, want the generated message", msg)
	}
}

// TestWriteMessageFile_DoesNotFollowSymlink pins the primitive directly: a
// symlink at the destination is replaced, never followed.
func TestWriteMessageFile_DoesNotFollowSymlink(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	target := filepath.Join(dir, "victim")

	const original = "original"

	if err := os.WriteFile(target, []byte(original), 0600); err != nil {
		t.Fatalf("write victim: %v", err)
	}

	link := filepath.Join(dir, "message")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := writeMessageFile(link, []byte("replacement")); err != nil {
		t.Fatalf("writeMessageFile: %v", err)
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read victim: %v", err)
	}

	if string(got) != original {
		t.Errorf("victim target was overwritten: got %q, want %q", got, original)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat message file: %v", err)
	}

	if !fi.Mode().IsRegular() {
		t.Errorf("message file mode = %v, want a regular file", fi.Mode())
	}

	msg, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("read message file: %v", err)
	}

	if string(msg) != "replacement" {
		t.Errorf("message file = %q, want %q", msg, "replacement")
	}
}

// TestWriteMessageFile_CreatesAndReplaces covers the two ordinary cases the
// hook relies on: a missing file is created and an existing file is replaced.
func TestWriteMessageFile_CreatesAndReplaces(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "COMMIT_EDITMSG")

	if err := writeMessageFile(path, []byte("first")); err != nil {
		t.Fatalf("writeMessageFile(create): %v", err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "first" {
		t.Fatalf("created file = %q, %v; want %q", got, err, "first")
	}

	if err := writeMessageFile(path, []byte("second")); err != nil {
		t.Fatalf("writeMessageFile(replace): %v", err)
	}

	if got, err := os.ReadFile(path); err != nil || string(got) != "second" {
		t.Fatalf("replaced file = %q, %v; want %q", got, err, "second")
	}
}
