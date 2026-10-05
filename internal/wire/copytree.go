package wire

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

// copyBufferSize bounds memory while copying file contents.
const copyBufferSize = 64 * 1024

// CopyTree copies the populated source directory into target, returning the
// regular-file count. Source is a local populated worktree subset (never a
// remote stream), so containment is structural: entries come from a walk,
// not from names. Only regular files are copied; symlinks and other
// non-regular entries are skipped and debug-logged — materializing upstream
// links could escape the target, and a link-free target keeps divergence
// detection exact. Total bytes copied are bounded by maxBytes.
func CopyTree(srcDir, target string, maxBytes int64) (int, error) {
	if maxBytes <= 0 {
		return 0, fmt.Errorf("non-positive copy budget %d", maxBytes)
	}

	info, err := os.Stat(srcDir)
	if err != nil {
		return 0, fmt.Errorf("inspect source: %w", err)
	}

	if !info.IsDir() {
		return 0, fmt.Errorf("source %s is not a directory", srcDir)
	}

	copier := &treeCopier{src: srcDir, dst: target, budget: maxBytes, buf: make([]byte, copyBufferSize)}

	if err := filepath.WalkDir(srcDir, copier.visit); err != nil {
		return copier.files, err
	}

	return copier.files, nil
}

// treeCopier carries copy state across one walk.
type treeCopier struct {
	src    string
	dst    string
	budget int64
	buf    []byte
	files  int
}

// visit copies one entry: directories are created, regular files streamed,
// anything else skipped with a debug record.
func (c *treeCopier) visit(path string, d fs.DirEntry, err error) error {
	if err != nil {
		return err
	}

	rel, err := filepath.Rel(c.src, path)
	if err != nil {
		return err
	}

	if rel == "." {
		return os.MkdirAll(c.dst, 0o750)
	}

	if d.IsDir() {
		return os.MkdirAll(filepath.Join(c.dst, rel), 0o755)
	}

	if !entryRegular(d, path) {
		slog.Debug("wire skipping non-regular copy entry", "path", rel)

		return nil
	}

	if err := c.copyFile(path, filepath.Join(c.dst, rel)); err != nil {
		return err
	}

	c.files++

	return nil
}

// copyFile streams one regular file, created with the source's permission
// bits so executables survive the copy. Modes come from stat (a variable),
// never literals, and umask still applies.
func (c *treeCopier) copyFile(src, dst string) error {
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

	metered := &budgetWriter{w: out, budget: &c.budget}

	if _, err := io.CopyBuffer(metered, in, c.buf); err != nil {
		_ = out.Close()

		return fmt.Errorf("copy file: %w", err)
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("close file: %w", err)
	}

	return nil
}

// entryRegular reports whether the entry itself is a regular file, without
// following symlinks. Unknown entry types fall back to stat. (HashDir
// deliberately still follows user-added links so they count as divergence.)
func entryRegular(d fs.DirEntry, path string) bool {
	if typ := d.Type(); typ != 0 {
		return typ.IsRegular()
	}

	return isHashable(path)
}

// budgetWriter passes writes through while enforcing a shared byte
// budget: reads past the budget fail instead of silently truncating,
// except when the stream ends exactly at the budget.
type budgetWriter struct {
	w      io.Writer
	budget *int64
}

func (b *budgetWriter) Write(p []byte) (int, error) {
	if *b.budget <= 0 {
		return 0, errors.New("copy exceeds budget")
	}

	if int64(len(p)) > *b.budget {
		p = p[:*b.budget]
	}

	n, err := b.w.Write(p)
	*b.budget -= int64(n)

	return n, err
}
