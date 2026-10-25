package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/wire"

	"github.com/spf13/cobra"
)

// subfolderFake resolves only the refs and subpaths it is told exist.
type subfolderFake struct {
	commit  string
	refs    map[string]bool
	subpath map[string]bool
}

func (f *subfolderFake) Resolve(_ context.Context, source wire.SourceRef) (wire.Resolution, error) {
	if !f.refs[source.Ref] {
		return wire.Resolution{}, fmt.Errorf("ref %q: %w", source.Ref, wire.ErrUnknownRef)
	}

	if !f.subpath[source.Subpath] {
		return wire.Resolution{}, fmt.Errorf("path %q: %w", source.Subpath, wire.ErrMissingPath)
	}

	return wire.Resolution{Commit: f.commit, Ref: source.Ref, Subpath: source.Subpath}, nil
}

func (f *subfolderFake) Materialize(
	_ context.Context, _ wire.SourceRef, _ wire.Resolution, dir string,
) (int, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return 0, err
	}

	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha"), 0o600); err != nil {
		return 0, err
	}

	return 1, nil
}

// subfolderCmd builds a fetch command with the flags fetchWith reads.
func subfolderCmd(t *testing.T, fetcher wire.Fetcher, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()

	cmd := &cobra.Command{Use: "wire"}

	cmd.Flags().StringP("target-path", "t", "", "target")
	cmd.Flags().Bool("force", false, "force")
	cmd.Flags().StringP("target-name", "n", "", "name")
	addWireSubfolderFlag(cmd)

	var out bytes.Buffer

	cmd.SetOut(&out)
	cmd.SetArgs(args)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return fetchWith(cmd, args, fetcher)
	}

	return cmd, &out
}

func TestFetchWithSubfolderFromGenericRepositoryURL(t *testing.T) {
	run := t.TempDir()
	t.Chdir(run)

	sha := strings.Repeat("a", 40)
	fake := &subfolderFake{
		commit:  sha,
		refs:    map[string]bool{"HEAD": true},
		subpath: map[string]bool{"path/sub": true},
	}

	cmd, out := subfolderCmd(t, fake,
		"https://github.com/owner/repo", "--subfolder", "path/sub", "-t", "out")

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	if !strings.Contains(out.String(), "Fetched github.com/owner/repo@HEAD:path/sub") {
		t.Fatalf("output = %q", out.String())
	}

	if _, err := os.Stat(filepath.Join(run, "out", "a.txt")); err != nil {
		t.Fatalf("target missing content: %v", err)
	}
}

func TestFetchWithGenericRepositoryURLRequiresSubfolder(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := &subfolderFake{
		commit:  strings.Repeat("a", 40),
		refs:    map[string]bool{"HEAD": true},
		subpath: map[string]bool{"path/sub": true},
	}

	cmd, _ := subfolderCmd(t, fake, "https://github.com/owner/repo", "-t", "out")

	if err := cmd.Execute(); !errors.Is(err, wire.ErrBadURL) {
		t.Fatalf("err = %v, want ErrBadURL", err)
	}
}

func TestFetchWithSubfolderRejectsUnsafePath(t *testing.T) {
	t.Chdir(t.TempDir())

	fake := &subfolderFake{
		commit:  strings.Repeat("a", 40),
		refs:    map[string]bool{"HEAD": true},
		subpath: map[string]bool{"path/sub": true},
	}

	cmd, _ := subfolderCmd(t, fake,
		"https://github.com/owner/repo", "--subfolder", "path/../sub", "-t", "out")

	if err := cmd.Execute(); !errors.Is(err, wire.ErrBadURL) {
		t.Fatalf("err = %v, want ErrBadURL", err)
	}
}

// TestFetchWithRepositoryRootURLRejected covers FR-003b end-to-end: the
// refusal states that whole-repository fetch is unsupported, shows the
// expected shape, and leaves no target or registry behind.
func TestFetchWithRepositoryRootURLRejected(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/owner/repo/tree/main",
		"https://github.com/owner/repo/tree/main/",
	} {
		t.Run(raw, func(t *testing.T) {
			run := t.TempDir()
			t.Chdir(run)

			fake := &subfolderFake{
				commit:  strings.Repeat("a", 40),
				refs:    map[string]bool{"main": true},
				subpath: map[string]bool{"path/sub": true},
			}

			cmd, out := subfolderCmd(t, fake, raw, "-t", "out")

			err := cmd.Execute()
			if !errors.Is(err, wire.ErrBadURL) {
				t.Fatalf("err = %v, want ErrBadURL", err)
			}

			for _, want := range []string{
				"whole-repository fetch is not supported",
				"expected shape:",
			} {
				if !strings.Contains(err.Error(), want) {
					t.Fatalf("err = %q, want it to contain %q", err, want)
				}
			}

			// Cobra echoes usage on error; the invariant is that no fetch
			// success claim was printed.
			if strings.Contains(out.String(), "Fetched") {
				t.Fatalf("refused fetch claimed success: %q", out.String())
			}

			for _, leftover := range []string{"out", wire.RegistryPath(run)} {
				if _, statErr := os.Stat(leftover); !errors.Is(statErr, os.ErrNotExist) {
					t.Fatalf("refused fetch left %s behind (stat err = %v)", leftover, statErr)
				}
			}
		})
	}
}

// TestFetchWithRootURLSubfolderFlagDoesNotRescue proves FR-003c's no-override
// rule: --subfolder applies only to reference-less repository URLs.
func TestFetchWithRootURLSubfolderFlagDoesNotRescue(t *testing.T) {
	run := t.TempDir()
	t.Chdir(run)

	fake := &subfolderFake{
		commit:  strings.Repeat("a", 40),
		refs:    map[string]bool{"main": true},
		subpath: map[string]bool{"path/sub": true},
	}

	cmd, _ := subfolderCmd(t, fake,
		"https://github.com/owner/repo/tree/main", "--subfolder", "path/sub", "-t", "out")

	err := cmd.Execute()
	if !errors.Is(err, wire.ErrBadURL) {
		t.Fatalf("err = %v, want ErrBadURL", err)
	}

	if !strings.Contains(err.Error(), "whole-repository fetch is not supported") {
		t.Fatalf("err = %q, want whole-repository guidance", err)
	}

	if _, statErr := os.Stat(filepath.Join(run, "out")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("refused fetch left a target behind (stat err = %v)", statErr)
	}
}
