package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Aggregate content hashing for divergence detection.

// hashBufferSize bounds memory while hashing file contents.
const hashBufferSize = 64 * 1024

// HashDir returns the aggregate SHA-256 of dir: each regular file's content
// hashed under its slash-separated relative path, sorted, then hashed as a
// whole. The registry file itself is bookkeeping, never content, so it is
// excluded (a fetch with `-t .` keeps the registry inside its own target).
// Output is 64 lowercase hex chars.
func HashDir(dir string) (string, error) {
	paths, err := listFiles(dir)
	if err != nil {
		return "", err
	}

	sum, err := sumFiles(dir, paths)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(sum), nil
}

// listFiles returns sorted slash-separated relative paths of hashable files.
func listFiles(dir string) ([]string, error) {
	var paths []string

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		rel = filepath.ToSlash(rel)

		if rel == registryFileName {
			return nil
		}

		// Fast path: WalkDir already reports the entry type on most filesystems,
		// so a known-regular file skips the extra stat isHashable would do
		// (one newfstatat per file). Symlinks still stat the target — a link
		// to a regular file counts — and unknown types fall back to stat,
		// exactly matching the previous behavior.
		switch typ := d.Type(); {
		case typ.IsRegular():
		case typ == 0 || typ&fs.ModeSymlink != 0:
			if !isHashable(path) {
				return nil
			}
		default:
			return nil
		}

		paths = append(paths, rel)

		return nil
	}

	if err := filepath.WalkDir(dir, walk); err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	sort.Strings(paths)

	return paths, nil
}

// isHashable reports whether path is a regular file, following symlinks.
// Sockets, fifos, devices, and dangling links are skipped: reading them
// could block or fail for reasons unrelated to content.
func isHashable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	return info.Mode().IsRegular()
}

// sumFiles hashes each file's content under its relative path.
func sumFiles(dir string, paths []string) ([]byte, error) {
	h := sha256.New()
	buf := make([]byte, hashBufferSize)

	for _, rel := range paths {
		if _, err := io.WriteString(h, rel+"\x00"); err != nil {
			return nil, fmt.Errorf("hash name %s: %w", rel, err)
		}

		if err := hashFile(h, filepath.Join(dir, rel), buf); err != nil {
			return nil, err
		}

		if _, err := h.Write([]byte{0}); err != nil {
			return nil, fmt.Errorf("hash frame %s: %w", rel, err)
		}
	}

	return h.Sum(nil), nil
}

// hashFile streams one file's content into h using the shared buffer.
func hashFile(h io.Writer, path string, buf []byte) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}

	defer func() { _ = f.Close() }()

	if _, err := io.CopyBuffer(h, f, buf); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}

	return nil
}
