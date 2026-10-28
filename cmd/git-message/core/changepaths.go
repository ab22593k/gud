package core

import (
	"context"
	"log/slog"

	"gud/internal/request"
)

// resolveChangePaths runs one optional change-path probe and returns its
// paths, or nil when the probe fails. A failure emits exactly one debug
// record naming the shape and the cause; it never returns an error or a
// user-facing notice — the probe widens evidence, it never gates progress,
// so callers fall back to diff-derived paths (D9, Q3, FR-030).
func resolveChangePaths(probe func() ([]string, error)) []string {
	paths, err := probe()
	if err != nil {
		slog.Debug("change-path probe failed; using diff-derived paths", "error", err)

		return nil
	}

	return paths
}

// resolveStagedChange acquires the staged change exactly once: the diff via
// the existing staged-diff acquisition (deleted-context appended, with its
// truncation flag) and the paths via the app's staged probe. It returns the
// Change the request layer verifies and the truncation flag for the caller's
// notice. A probe failure yields Paths == nil with the same diff — never an
// error, never a notice.
func resolveStagedChange(ctx context.Context, app *AppContext) (request.Change, bool, error) {
	diff, truncated, err := getStagedDiffOrError(ctx)
	if err != nil {
		return request.Change{}, false, err
	}

	paths := resolveChangePaths(func() ([]string, error) { return app.stagedChangePaths(ctx) })

	return request.Change{Diff: diff, Paths: paths}, truncated, nil
}
