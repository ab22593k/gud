package wire

import (
	"context"
)

// Fetcher seam: the narrow boundary orchestration tests script against.

// Resolution is a fully disambiguated source: the commit to export plus
// the actual ref and subpath (which may differ from the greedy parse when
// refs contain slashes).
type Resolution struct {
	Commit  string
	Ref     string
	Subpath string
}

// Fetcher resolves references and materializes subfolders. The real
// implementation shells out to git (sparse-checkout worktree + copy-out);
// tests use scripted fakes so the suite stays deterministic with no
// network and no credentials.
//
// Resolve errors are classified: ErrUnknownRef (no matching ref),
// ErrMissingPath (ref resolves but holds no subfolder), or ErrUpstream
// (transport failure). Materialize errors are ErrMissingPath (commit lacks
// the subpath) or ErrUpstream.
type Fetcher interface {
	// Resolve returns the commit and actual ref/subpath a source points at.
	Resolve(ctx context.Context, source SourceRef) (Resolution, error)
	// Materialize populates dir with the subfolder at res, returning the
	// regular-file count.
	Materialize(ctx context.Context, source SourceRef, res Resolution, dir string) (int, error)
}
