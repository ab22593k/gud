package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// File-level three-way merge for updates (research D10, data-model §8).
// Paths compare by state across base (recorded commit), local (live
// target), and new (resolved commit) snapshots; content equality is by
// hash, so snapshots stay small regardless of file sizes.

// fileKind distinguishes absent, regular, and other (symlink, dir,
// device) paths. kindAbsent is the zero value so a missing map entry
// reads as absent.
type fileKind int

const (
	kindAbsent fileKind = iota
	kindRegular
	kindOther
)

// snapFile is one path's state in one tree snapshot. Regular files carry
// a content hash; anything else compares by kind alone (two links with
// different targets read as equal — keeping local is the safe choice).
type snapFile struct {
	kind  fileKind
	token string
}

// changedFrom reports whether f differs from the base state.
func (f snapFile) changedFrom(base snapFile) bool {
	return f.kind != base.kind || f.kind == kindRegular && f.token != base.token
}

// snapshotDir maps slash-separated relative paths to file states,
// excluding the registry file itself (it changes on every update and
// must never count as a local edit). Regular files hash content;
// symlinks and other non-regular entries record kind only — never
// followed, never read.
func snapshotDir(dir string) (map[string]snapFile, error) {
	out := make(map[string]snapFile)

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return nil
		}

		rel = filepath.ToSlash(rel)

		if rel == registryFileName {
			return nil
		}

		if d.IsDir() {
			return nil
		}

		if !entryRegular(d, path) {
			out[rel] = snapFile{kind: kindOther}

			return nil
		}

		sum, err := hashContent(path)
		if err != nil {
			return err
		}

		out[rel] = snapFile{kind: kindRegular, token: sum}

		return nil
	}

	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, fmt.Errorf("snapshot %s: %w", dir, err)
	}

	return out, nil
}

// hashContent returns the hex SHA-256 of a regular file.
func hashContent(path string) (string, error) {
	h := sha256.New()

	if err := hashFile(h, path, make([]byte, copyBufferSize)); err != nil {
		return "", err
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// classify compares base/local/new snapshots per the data-model §8
// matrix, returning upstream-take paths, clean-delete paths, and
// conflicts. Slices stay nil when empty.
func classify(base, local, newer map[string]snapFile) (take, del, conflicts []string) {
	for _, p := range unionKeys(base, local, newer) {
		b, l, n := base[p], local[p], newer[p]

		switch {
		case !l.changedFrom(b) && !n.changedFrom(b):
			// Untouched everywhere: keep.
		case n.changedFrom(b) && !l.changedFrom(b):
			// Upstream-only change.
			switch n.kind {
			case kindRegular:
				take = append(take, p)
			case kindAbsent:
				del = append(del, p)
			default:
				// Upstream replaced with non-regular: skip
				// fetch-consistently rather than merging a link.
			}
		case l.changedFrom(b) && !n.changedFrom(b):
			// Local-only change (including local deletes): keep.
		case l == n:
			// Convergent edits (including both deleted): keep.
		default:
			conflicts = append(conflicts, p)
		}
	}

	return take, del, conflicts
}

// unionKeys returns the sorted union of snapshot paths for deterministic
// classification (and stable conflict messages).
func unionKeys(maps ...map[string]snapFile) []string {
	seen := make(map[string]bool)

	var out []string

	for _, m := range maps {
		for p := range m {
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
		}
	}

	sort.Strings(out)

	return out
}

// applyMerge overlays take paths from newDir and removals onto staging,
// which the caller cloned from the target. Take contents copy with
// upstream modes; deletions remove (already-gone targets are skipped).
func applyMerge(staging, newDir string, take, del []string) error {
	for _, p := range take {
		src := filepath.Join(newDir, filepath.FromSlash(p))
		dst := filepath.Join(staging, filepath.FromSlash(p))

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create parent for %s: %w", p, err)
		}

		if err := copyOneFile(src, dst); err != nil {
			return fmt.Errorf("take %s: %w", p, err)
		}
	}

	for _, p := range del {
		full := filepath.Join(staging, filepath.FromSlash(p))

		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("delete %s: %w", p, err)
		}
	}

	return nil
}

// copyOneFile copies one regular file preserving the source mode. Inputs
// already passed through budgeted materialization, so no second budget
// applies here (see CopyTree for the budgeted bulk path).
func copyOneFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}

	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()

		return fmt.Errorf("copy file: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}

// cloneCheckout copies target into a fresh staging sibling for merge
// application, preserving symlinks as links. Sibling placement keeps
// relative links resolving identically, so no link semantics change.
func cloneCheckout(target, staging string) error {
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(target, path)
		if err != nil {
			return err
		}

		if rel == "." {
			return os.MkdirAll(staging, 0o750)
		}

		dst := filepath.Join(staging, rel)

		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}

		if d.Type()&fs.ModeSymlink != 0 {
			return cloneLink(path, dst)
		}

		if !d.Type().IsRegular() {
			if d.Type() != 0 {
				slog.Debug("wire skipping non-regular clone entry", "path", rel)

				return nil
			}

			if !isHashable(path) {
				slog.Debug("wire skipping non-regular clone entry", "path", rel)

				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create parent: %w", err)
		}

		return copyOneFile(path, dst)
	}

	if err := filepath.WalkDir(target, walk); err != nil {
		return fmt.Errorf("clone checkout: %w", err)
	}

	return nil
}

// cloneLink recreates one symlink at dst with the same target.
func cloneLink(src, dst string) error {
	aim, err := os.Readlink(src)
	if err != nil {
		return fmt.Errorf("read link: %w", err)
	}

	if err := os.Symlink(aim, dst); err != nil {
		return fmt.Errorf("write link: %w", err)
	}

	return nil
}
