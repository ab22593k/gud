package wire

// Wire cache locations and checkout record paths.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// recordFileName is the colocated tracking record stored in every checkout.
const recordFileName = ".git-wire.json"

// Store resolves filesystem locations for shared mirrors and checkouts.
// The zero value is not usable; use NewStore or NewStoreWithDir.
type Store struct {
	root string
}

// NewStore returns a Store rooted at the default cache directory,
// ~/.config/gud/wire, creating it when absent.
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}

	return NewStoreWithDir(filepath.Join(home, ".config", "gud", "wire")), nil
}

// NewStoreWithDir returns a Store rooted at dir, creating it when absent.
// It is the test hook for hermetic filesystem isolation.
func NewStoreWithDir(dir string) *Store {
	_ = os.MkdirAll(dir, 0o750)

	return &Store{root: dir}
}

// Root reports the cache directory the store was created with.
func (s *Store) Root() string {
	return s.root
}

// MirrorDir returns the shared mirror directory for one repository.
// Segments are sanitized so URL-derived values can never escape the root.
func (s *Store) MirrorDir(host, owner, repo string) string {
	return filepath.Join(s.root, "repos", safeSegment(host), safeSegment(owner), safeSegment(repo))
}

// RecordPath returns the tracking record path inside checkout dir.
func RecordPath(dir string) string {
	return filepath.Join(dir, recordFileName)
}

// WorktreeDir mints a unique ephemeral worktree directory under the cache
// root for one materialization. The caller owns its removal.
func (s *Store) WorktreeDir() (string, error) {
	parent := filepath.Join(s.root, "worktrees")

	if err := os.MkdirAll(parent, 0o750); err != nil {
		return "", fmt.Errorf("stage worktree parent: %w", err)
	}

	dir, err := os.MkdirTemp(parent, "wt-*")
	if err != nil {
		return "", fmt.Errorf("stage worktree dir: %w", err)
	}

	return dir, nil
}

// safeSegment maps one path segment into a traversal-proof form.
// Parse-time validation (ParseSourceURL) is the real gate; this is
// defense-in-depth so a segment can never become absolute, dot, or nested.
func safeSegment(seg string) string {
	var b strings.Builder

	for _, r := range seg {
		if isSegmentRune(r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}

	out := strings.Trim(b.String(), "._")
	if out == "" {
		return "_"
	}

	return out
}

// isSegmentRune reports whether r may appear unescaped in a cache segment.
func isSegmentRune(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
}
