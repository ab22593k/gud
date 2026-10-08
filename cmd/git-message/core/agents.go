package core

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	// agentsFileName is the instruction file mounted into the agent's
	// environment. The runtime loads it as system instructions on startup, so
	// a repository's own agent conventions reach the model without being
	// pasted into the prompt.
	agentsFileName = "AGENTS.md"

	// maxAgentsFileBytes caps how much of AGENTS.md is mounted. The content
	// travels inline in the request body, and an unbounded file would turn a
	// stray generated artifact into a rejected API call. Oversized files are
	// skipped rather than truncated: silently cutting instructions in half
	// would drop exactly the rules a large file is most likely to hold.
	maxAgentsFileBytes = 64 << 10
)

// resolveAgentInstructions returns the AGENTS.md content to mount into the
// agent's environment, or "" when there is nothing usable to mount.
//
// Search order is the invocation directory first, then the repository root: a
// file next to the changes being committed is more specific than one at the
// root, and running from a subdirectory should not silently lose the root
// instructions. Both lookups are best-effort — a commit must not fail because
// an optional instruction file is missing or unreadable.
func resolveAgentInstructions(ctx context.Context, app *AppContext) string {
	cwd, err := os.Getwd()
	if err != nil {
		slog.Debug("AGENTS.md lookup skipped: no working directory", "error", err)

		return ""
	}

	if content, ok := readAgentsFile(cwd); ok {
		return content
	}

	root, err := app.RepoRoot(ctx)
	if err != nil || root == "" {
		slog.Debug("AGENTS.md lookup skipped: no repository root", "error", err)

		return ""
	}

	// The invocation directory was already checked; comparing cleaned paths
	// avoids a second stat when they are the same directory.
	if filepath.Clean(root) == filepath.Clean(cwd) {
		return ""
	}

	content, _ := readAgentsFile(root)

	return content
}

// readAgentsFile reads dir/AGENTS.md and reports whether it yielded usable
// instructions.
//
// Every failure mode is non-fatal and logged, never returned: the caller has no
// way to act on it, and an unreadable instruction file must not block a commit.
// A missing file is the common case and stays at debug level so a repository
// without one produces no noise.
func readAgentsFile(dir string) (string, bool) {
	path := filepath.Join(dir, agentsFileName)

	info, err := os.Stat(path)
	if err != nil {
		// Covers a plain miss, a broken symlink, and a permission failure on
		// the parent directory: none of them is worth more than a debug line.
		slog.Debug("AGENTS.md not readable", "path", path, "error", err)

		return "", false
	}

	// A directory named AGENTS.md is a collision, not instructions. os.ReadFile
	// would reject it too, but this states the reason.
	if info.IsDir() {
		slog.Debug("AGENTS.md is a directory; ignoring", "path", path)

		return "", false
	}

	data, err := os.ReadFile(path) //nolint:gosec // G304: path is dir + a fixed filename
	if err != nil {
		// The stat succeeded, so this is a race (unlinked between the two
		// calls), a permission failure on the file itself, or an I/O error.
		slog.Warn("AGENTS.md unreadable; proceeding without it", "path", path, "error", err)

		return "", false
	}

	if !utf8.Valid(data) {
		// The request body is JSON; invalid UTF-8 would be rejected by the
		// encoder rather than by the API, with a far less useful message.
		slog.Warn("AGENTS.md is not valid UTF-8; proceeding without it", "path", path)

		return "", false
	}

	// Checked on what was actually read, not on the stat size: the file can
	// grow between the two calls.
	if len(data) > maxAgentsFileBytes {
		slog.Warn("AGENTS.md is too large to mount; proceeding without it",
			"path", path, "bytes", len(data), "max_bytes", maxAgentsFileBytes)

		return "", false
	}

	// Strip a UTF-8 BOM (editors on Windows add one) and surrounding
	// whitespace, then treat an all-whitespace file as absent: mounting an
	// empty AGENTS.md would mount a file with nothing in it.
	content := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if content == "" {
		slog.Debug("AGENTS.md is empty", "path", path)

		return "", false
	}

	slog.Debug("AGENTS.md mounted", "path", path, "bytes", len(content))

	return content, true
}
