package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// This file is the single place where gud spawns git. Every exported helper in
// the package goes through runGit so subprocess construction, output capture,
// and error wrapping are defined once rather than repeated per query.

// newGitCmd builds a git command bound to ctx. Callers set the fields they need
// (Dir, Stdin, Env) and run it, or they use one of the runGit* helpers below.
// Arguments come from internal callers (git output, resolved revisions,
// repo-relative paths), never from remote or untrusted input.
//
//nolint:gosec // G204: the binary is always the fixed "git" command.
func newGitCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "git", args...)
}

// runGit runs a git command and returns its stdout verbatim. stderr is folded
// into stdout so a failure's diagnostics survive into the wrapped error, which
// is what git command failures are almost always about.
func runGit(ctx context.Context, args ...string) (string, error) {
	cmd := newGitCmd(ctx, args...)

	var out bytes.Buffer

	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w\n%s", args, err, out.String())
	}

	return out.String(), nil
}

// runGitTrimmed runs a git command and returns stdout with surrounding
// whitespace removed, the normal form for single-value queries (a path, a
// revision, a branch name).
func runGitTrimmed(ctx context.Context, args ...string) (string, error) {
	out, err := runGit(ctx, args...)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// runGitQuietly runs a git command and returns trimmed stdout, or "" on failure.
// It is for read-only queries whose failure should degrade the result rather
// than fail the operation: callers either have a fallback or treat empty as
// "no information available".
func runGitQuietly(ctx context.Context, args ...string) string {
	out, err := runGitTrimmed(ctx, args...)
	if err != nil {
		return ""
	}

	return out
}

// runGitDir is runGit with an explicit working directory, so relative arguments
// (log pathspecs, for instance) resolve against dir rather than the caller's
// cwd.
func runGitDir(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := newGitCmd(ctx, args...)
	cmd.Dir = dir

	var out bytes.Buffer

	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v in %s: %w", args, dir, err)
	}

	return strings.TrimSpace(out.String()), nil
}

// runGitStdin runs a git command feeding message on stdin and returns stdout
// verbatim. It backs the commands that transform text piped through git, such as
// interpret-trailers.
func runGitStdin(ctx context.Context, message string, args ...string) (string, error) {
	cmd := newGitCmd(ctx, args...)
	cmd.Stdin = bytes.NewBufferString(message)

	var out bytes.Buffer

	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %v: %w", args, err)
	}

	return out.String(), nil
}

// gitConfigGlobal runs `git config --global --get <key>` and reports whether
// the key was set. The two callers differ in what an unset key means — a
// missing hooks path is fine, a foreign one must not be overwritten — so the
// distinction is preserved instead of collapsed into an empty string.
func gitConfigGlobal(ctx context.Context, key string) (value string, set bool, err error) {
	out, err := runGitTrimmed(ctx, "config", "--global", "--get", key)
	if err == nil {
		return out, true, nil
	}

	if isGitExitCode(err, gitExitCodeUnset) {
		return "", false, nil
	}

	return "", false, fmt.Errorf("read global %s: %w", key, err)
}

const (
	// gitExitCodeUnset is git config's exit code for "key not present".
	gitExitCodeUnset = 1
	// gitExitCodeMissingValue is git config's exit code for --unset on a key
	// that is not present.
	gitExitCodeMissingValue = 5
)

// isGitExitCode reports whether err is a git invocation that exited with code,
// which is how git signals "no" as opposed to a genuine failure.
func isGitExitCode(err error, code int) bool {
	var exitErr *exec.ExitError

	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}
