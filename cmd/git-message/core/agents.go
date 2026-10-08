package core

import (
	"context"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"gud/internal/detect"
	"gud/internal/request"
)

const (
	// agentsFileName is the instruction file mounted into the agent's
	// environment. In a monorepo each subproject ships its own, and the
	// closest one to the invocation directory wins.
	agentsFileName = "AGENTS.md"

	// maxAgentsFileBytes caps how much of a single AGENTS.md is mounted. The
	// content travels inline in the request body, and an unbounded file would
	// turn a stray generated artifact into a rejected API call. Oversized
	// files are skipped rather than truncated: silently cutting instructions
	// in half would drop exactly the rules a large file is most likely to hold.
	maxAgentsFileBytes = 64 << 10

	// maxAgentsFiles caps how many files are mounted. Every one is inline
	// content in the same request body, so a monorepo deeper than this keeps
	// only its nearest files.
	maxAgentsFiles = 32

	// maxAgentsTotalBytes caps the combined payload across all mounted files,
	// so many small files cannot add up to an oversized request.
	maxAgentsTotalBytes = 256 << 10

	// maxAgentDirs bounds the downward scan. A commit must not pay a full
	// repository walk to find instruction files.
	maxAgentDirs = 2000
)

// agentSkipDirs are never descended into while scanning for nested files. They
// hold dependencies and build output, which are not source trees and can be
// enormous; .gitignore pruning covers the rest.
var agentSkipDirs = map[string]bool{
	".git":         true,
	".cache":       true,
	"__pycache__":  true,
	"build":        true,
	"dist":         true,
	"node_modules": true,
	"target":       true,
	"vendor":       true,
}

// resolveAgentFiles returns the AGENTS.md files to mount into the agent's
// environment, ordered nearest first.
//
// Discovery walks up from the invocation directory to the repository root —
// that chain is the precedence order, so the closest file comes first — and
// then walks down from the same directory, bounded, so every subproject below
// can ship a file too.
//
// The nearest file is mounted at request.RootMountPath, the slot the Antigravity
// runtime loads as system instructions; the rest keep their
// repository-relative paths so the tree they describe exists in the sandbox
// for the agent to read.
//
// Every failure mode is non-fatal and logged, never returned: an optional
// instruction file must not block a commit.
func resolveAgentFiles(ctx context.Context, app *AppContext) []request.AgentFile {
	start, err := os.Getwd()
	if err != nil {
		slog.Debug("AGENTS.md lookup skipped: no working directory", "error", err)

		return nil
	}

	root, err := app.RepoRoot(ctx)
	if err != nil || root == "" {
		// Still worth scanning: a directory outside any repository can hold an
		// AGENTS.md, and there is no root to anchor the upward walk at.
		slog.Debug("AGENTS.md lookup: no repository root", "error", err)

		root = ""
	}

	// An invocation directory the repository does not contain (a sibling
	// checkout, a bare path) would otherwise find nothing: fall back to the
	// repository root so the commit still picks up its instructions.
	start = discoveryStart(start, root)

	slog.Debug("AGENTS.md discovery start", "cwd", start, "root", root)

	return agentFilesFrom(ctx, start, root)
}

// discoveryStart resolves where discovery should begin: the invocation
// directory when the repository contains it, the repository root otherwise.
// Without a root there is nothing to fall back to, so the invocation directory
// is its own anchor.
func discoveryStart(cwd, root string) string {
	if root != "" && !isWithin(root, cwd) {
		return root
	}

	return cwd
}

// agentFilesFrom discovers and mounts the AGENTS.md files reachable from start,
// looking in directories ordered nearest first. root is the repository root
// when one is known; when it is not, the nearest ancestor holding .git plays
// that role, and when there is neither, start is its own anchor.
func agentFilesFrom(ctx context.Context, start, root string) []request.AgentFile {
	anchor := upRoot(start, root)

	var files []request.AgentFile

	var total int

	for _, dir := range discoveredDirs(ctx, start, anchor) {
		content, ok := readAgentsFile(dir)
		if !ok {
			continue
		}

		if len(files) >= maxAgentsFiles || total+len(content) > maxAgentsTotalBytes {
			slog.Debug("AGENTS.md mount budget reached; ignoring the rest",
				"mounted_files", len(files), "mounted_bytes", total,
				"max_files", maxAgentsFiles, "max_bytes", maxAgentsTotalBytes)

			break
		}

		at, ok := mountPath(anchor, dir, len(files) == 0)
		if !ok {
			continue
		}

		total += len(content)
		files = append(files, request.AgentFile{Path: at, Content: content})
	}

	slog.Debug("AGENTS.md files resolved", "files", len(files), "bytes", total, "anchor", anchor)

	return files
}

// upRoot resolves the anchor the upward walk stops at and mount paths are
// relative to.
//
// The repository root reported by git is authoritative when it is available.
// When it is not — git is missing, or the lookup fails for any other reason —
// the nearest ancestor holding .git is used instead, so a repository that
// cannot be resolved still gets its ancestor files rather than silently losing
// all of them. With no repository at all, start is its own anchor.
func upRoot(start, root string) string {
	if root != "" && isWithin(root, start) {
		return root
	}

	for dir := start; ; {
		if isGitDir(dir) {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return start
		}

		dir = parent
	}
}

// isGitDir reports whether dir looks like a repository root: it holds a .git
// entry, which is a directory in a normal clone and a file in a worktree or
// submodule.
func isGitDir(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))

	return err == nil
}

// mountPath is the sandbox location for the file in dir.
//
// The first file mounted is the nearest one and takes the environment root,
// which the runtime loads as system instructions. The rest keep their
// repository-relative paths, except the repository-root file itself: its
// natural path is the root slot the nearest file already claimed, and the
// closer file supersedes it.
func mountPath(root, dir string, primary bool) (string, bool) {
	if primary {
		return request.RootMountPath, true
	}

	if root == "" {
		return "", false
	}

	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return "", false
	}

	rel = filepath.ToSlash(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}

	return path.Join(rel, agentsFileName), true
}

// discoveredDirs returns the directories to look in, nearest first: the walk
// from start up to root, followed by the bounded walk down from start.
func discoveredDirs(ctx context.Context, start, root string) []string {
	var dirs []string

	seen := make(map[string]bool)

	add := func(dir string) {
		dir = filepath.Clean(dir)
		if seen[dir] {
			return
		}

		seen[dir] = true
		dirs = append(dirs, dir)
	}

	if root != "" && isWithin(root, start) {
		// Ancestors, nearest first. This chain is the precedence order.
		for dir := start; ; dir = filepath.Dir(dir) {
			add(dir)

			if dir == root || filepath.Dir(dir) == dir {
				break
			}
		}
	} else {
		// Nothing to walk up to: the invocation directory is all there is.
		add(start)
	}

	walkDown(ctx, start, root, add)

	return dirs
}

// walkDown collects directories below start, pruning .gitignore'd trees and
// known dependency directories so the scan stays cheap on a large monorepo.
func walkDown(ctx context.Context, start, root string, add func(string)) {
	var matcher *detect.GitignoreMatcher
	if root != "" {
		matcher = detect.LoadGitignore(root)
	}

	visited := 0

	_ = filepath.WalkDir(start, func(p string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Unreadable paths are skipped silently: an instruction file is
		// optional and a permissions failure is not a reason to fail a commit.
		if err != nil {
			return nil //nolint:nilerr // documented: unreadable paths are skipped
		}

		if !d.IsDir() {
			return nil
		}

		if visited >= maxAgentDirs {
			return filepath.SkipAll
		}

		visited++

		if p != start {
			if agentSkipDirs[d.Name()] {
				return filepath.SkipDir
			}

			if matcher != nil && matcher.Ignored(relToRoot(root, p)) {
				return filepath.SkipDir
			}
		}

		add(p)

		return nil
	})
}

// relToRoot is the slash-separated path of p relative to root, or "" when it
// cannot be computed. A matcher that cannot place a path treats it as ignored,
// so the failure is reported as no-match instead.
func relToRoot(root, p string) string {
	if root == "" {
		return ""
	}

	rel, err := filepath.Rel(root, p)
	if err != nil {
		return ""
	}

	return filepath.ToSlash(rel)
}

// isWithin reports whether p is root itself or somewhere below it.
func isWithin(root, p string) bool {
	rel := relToRoot(root, p)

	return rel != "" && rel != ".." && !strings.HasPrefix(rel, "../")
}

// readAgentsFile reads dir/AGENTS.md and reports whether it yielded usable
// instructions.
//
// Every failure mode is non-fatal and logged, never returned: the caller has no
// way to act on it, and an unreadable instruction file must not block a commit.
// A missing file is the common case and stays at debug level so a repository
// without one produces no noise.
func readAgentsFile(dir string) (string, bool) {
	p := filepath.Join(dir, agentsFileName)

	info, err := os.Stat(p)
	if err != nil {
		// Covers a plain miss, a broken symlink, and a permission failure on
		// the parent directory: none of them is worth more than a debug line.
		slog.Debug("AGENTS.md not readable", "path", p, "error", err)

		return "", false
	}

	// A directory named AGENTS.md is a collision, not instructions. os.ReadFile
	// would reject it too, but this states the reason.
	if info.IsDir() {
		slog.Debug("AGENTS.md is a directory; ignoring", "path", p)

		return "", false
	}

	data, err := os.ReadFile(p) //nolint:gosec // G304: path is dir + a fixed filename
	if err != nil {
		// The stat succeeded, so this is a race (unlinked between the two
		// calls), a permission failure on the file itself, or an I/O error.
		slog.Warn("AGENTS.md unreadable; proceeding without it", "path", p, "error", err)

		return "", false
	}

	if !utf8.Valid(data) {
		// The request body is JSON; invalid UTF-8 would be rejected by the
		// encoder rather than by the API, with a far less useful message.
		slog.Warn("AGENTS.md is not valid UTF-8; proceeding without it", "path", p)

		return "", false
	}

	if len(data) > maxAgentsFileBytes {
		slog.Warn("AGENTS.md is too large to mount; proceeding without it",
			"path", p, "bytes", len(data), "max_bytes", maxAgentsFileBytes)

		return "", false
	}

	// Strip a UTF-8 BOM (editors on Windows add one) and surrounding
	// whitespace, then treat an all-whitespace file as absent: mounting an
	// empty AGENTS.md would mount a file with nothing in it.
	content := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	if content == "" {
		slog.Debug("AGENTS.md is empty", "path", p)

		return "", false
	}

	return content, true
}
