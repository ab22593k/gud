package git

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// StagedChangePaths reports the repo-relative paths the staged change
// touches, one entry per NUL-delimited --name-status record: one path for
// plain records, both sides for renames and copies. It runs over
// `git diff --cached --name-status -z -M -C` with rename and copy detection
// passed explicitly so a repository `diff.renames=false` cannot suppress the
// rename records, and without `--find-copies-harder` (its cost is unbounded;
// a pure copy is reported as an add, whose path still enters the set).
//
// The probe is evidence for file-grounding, not the evidence itself: it
// names the same change the diff patch describes, including pre-images the
// patch may omit (renames read as delete+add, cut diffs). Callers resolve it
// once per invocation and fall back to diff-derived paths on error.
func StagedChangePaths(ctx context.Context) ([]string, error) {
	out, err := runGitOutput(ctx, "diff", "--cached", "--name-status", "-z", "-M", "-C")
	if err != nil {
		return nil, err
	}

	return parseChangePaths(out)
}

// CommitChangePaths reports the repo-relative paths the commit sha touches,
// with the same record shape as StagedChangePaths, over
// `git show --format= --name-status -z -M -C <sha> --`: the commit's own
// patch only, never its parent's.
func CommitChangePaths(ctx context.Context, sha string) ([]string, error) {
	out, err := runGitOutput(ctx, "show", "--format=", "--name-status", "-z", "-M", "-C", sha, "--")
	if err != nil {
		return nil, err
	}

	return parseChangePaths(out)
}

// parseChangePaths interprets one NUL-delimited --name-status stream: the
// first field of each record is the status and consumes two paths for
// renames and copies (with or without a similarity score suffix) and one
// path otherwise. The stream ends with a single terminating NUL, whose
// trailing empty field is ignored; any other non-empty leftover is a shape
// error naming the record shape, never the path content. Empty input yields
// a non-nil empty slice so callers can reserve nil for "probe unavailable".
func parseChangePaths(out []byte) ([]string, error) {
	fields := strings.Split(string(out), "\x00")

	// Drop the single trailing empty field the terminating NUL produces.
	if n := len(fields); n > 0 && fields[n-1] == "" {
		fields = fields[:n-1]
	}

	paths := make([]string, 0)

	for i := 0; i < len(fields); {
		status := fields[i]
		i++

		var want int

		switch {
		case status == "R" || status == "C":
			want = 2
		case len(status) > 0 && (status[0] == 'R' || status[0] == 'C'):
			want = 2
		case len(status) > 0 && strings.ContainsRune("MADTUX", rune(status[0])):
			want = 1
		case i == len(fields):
			return nil, errors.New("parse change paths: trailing non-empty field, want terminating NUL")
		default:
			return nil, errors.New("parse change paths: unknown status in record")
		}

		if len(fields)-i < want {
			return nil, fmt.Errorf("parse change paths: short record, want %d path(s)", want)
		}

		paths = append(paths, fields[i:i+want]...)

		i += want
	}

	return paths, nil
}
