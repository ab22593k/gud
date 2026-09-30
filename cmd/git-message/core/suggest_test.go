package core

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gud/internal/config"
	"gud/internal/profile"
)

// suggestionsForTest returns a one-entry catalog matching the suggestion
// fixture used across handleProfileSelection tests.
func suggestionsForTest() []profile.CatalogEntry {
	return []profile.CatalogEntry{
		{Slug: "astrophysicist", Profession: "Astrophysicist", WorkMode: "physics"},
	}
}

// TestFetchSuggestions_RanksStubbedCatalog verifies the rank path end to end
// with a stubbed catalogFn: a Python-only temp repo produces stats whose top
// extension ranks the matching catalog entry first, with no network.
//
// Not parallel: it swaps the package-level catalogFn.
func TestFetchSuggestions_RanksStubbedCatalog(t *testing.T) {
	orig := catalogFn

	t.Cleanup(func() { catalogFn = orig })

	catalogFn = func(context.Context) ([]profile.CatalogEntry, error) {
		return []profile.CatalogEntry{
			{Slug: "python-dev", Profession: "Python Developer", Summary: "writes python code"},
			{Slug: "rust-dev", Profession: "Rust Developer", Summary: "writes rust code"},
		}, nil
	}

	repo := t.TempDir()

	for _, name := range []string{"main.py", "util.py"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte("print(1)\n"), 0600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, err := fetchSuggestions(t.Context(), repo)
	if err != nil {
		t.Fatalf("fetchSuggestions: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("fetchSuggestions() returned %d entries, want 1:\n%+v", len(got), got)
	}

	if got[0].Slug != "python-dev" {
		t.Errorf("top suggestion = %q, want %q", got[0].Slug, "python-dev")
	}
}

// TestFetchSuggestions_CatalogError verifies a catalog failure is wrapped
// with "fetch catalog" context rather than silently returning nothing.
func TestFetchSuggestions_CatalogError(t *testing.T) {
	orig := catalogFn

	t.Cleanup(func() { catalogFn = orig })

	catalogFn = func(context.Context) ([]profile.CatalogEntry, error) {
		return nil, errors.New("network down")
	}

	repo := t.TempDir()

	if err := os.WriteFile(filepath.Join(repo, "x.py"), []byte("x\n"), 0600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := fetchSuggestions(t.Context(), repo)
	if err == nil {
		t.Fatal("fetchSuggestions() = nil error, want catalog failure wrapped")
	}

	if !strings.Contains(err.Error(), "fetch catalog") {
		t.Errorf("error = %v, want 'fetch catalog' context", err)
	}
}

// TestHandleProfileSelection_DispatchPaths covers the skip, abort, invalid,
// out-of-range, and closed-input paths. The cwd argument is a temp dir, so
// skip-marker writes are observable without touching the process CWD.
func TestHandleProfileSelection_DispatchPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantMarker bool
		wantOut    []string
	}{
		{
			name:       "empty line skips and writes marker",
			input:      "\n",
			wantMarker: true,
			wantOut:    []string{"Skipped."},
		},
		{
			name:       "s skips and writes marker",
			input:      "s\n",
			wantMarker: true,
			wantOut:    []string{"Skipped."},
		},
		{
			name:       "skip keyword is case-insensitive",
			input:      "SKIP\n",
			wantMarker: true,
			wantOut:    []string{"Skipped."},
		},
		{
			name:    "abort writes no marker",
			input:   "a\n",
			wantOut: []string{"Aborted."},
		},
		{
			name:       "out-of-range selection treated as skip",
			input:      "7\n",
			wantMarker: true,
			wantOut:    []string{`Invalid selection "7"`},
		},
		{
			name:       "zero selection treated as skip",
			input:      "0\n",
			wantMarker: true,
			wantOut:    []string{`Invalid selection "0"`},
		},
		{
			name:       "non-numeric input treated as skip",
			input:      "later\n",
			wantMarker: true,
			wantOut:    []string{`Invalid selection "later"`},
		},
		{
			name:       "closed input is a no-op",
			input:      "",
			wantMarker: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			cwd := t.TempDir()

			err := handleProfileSelection(t.Context(), &AppContext{}, &buf,
				strings.NewReader(tt.input), cwd, suggestionsForTest())
			if (err != nil) != tt.wantErr {
				t.Fatalf("handleProfileSelection() error = %v, wantErr %v", err, tt.wantErr)
			}

			if _, statErr := os.Stat(filepath.Join(cwd, skipMarker)); os.IsNotExist(statErr) == tt.wantMarker {
				t.Errorf("skip marker exists = %v, want %v", !tt.wantMarker, tt.wantMarker)
			}

			for _, want := range tt.wantOut {
				if !strings.Contains(buf.String(), want) {
					t.Errorf("output missing %q:\n  got: %q", want, buf.String())
				}
			}
		})
	}
}

// TestHandleProfileSelection_AppliesSelectedProfile verifies that a valid
// numeric selection applies the profile to the app config and persists it
// in gud.json in the caller-provided working directory.
//
// Not parallel: it swaps the package-level profileManager; all profileManager
// swap sites in this package are sequential for the same reason.
func TestHandleProfileSelection_AppliesSelectedProfile(t *testing.T) {
	orig := profileManager

	t.Cleanup(func() { profileManager = orig })

	profileManager = profile.NewManagerWithDir(t.TempDir())
	if err := profileManager.Save("astrophysicist", profile.Profile{
		Slug:    "astrophysicist",
		Content: "test content",
	}); err != nil {
		t.Fatalf("save profile: %v", err)
	}

	workDir := t.TempDir()

	var buf bytes.Buffer

	app := &AppContext{}

	if err := handleProfileSelection(t.Context(), app, &buf,
		strings.NewReader("1\n"), workDir, suggestionsForTest()); err != nil {
		t.Fatalf("handleProfileSelection() error = %v", err)
	}

	if got := app.cfg.Profile; got != config.ProfileName("astrophysicist") {
		t.Errorf("app.cfg.Profile = %q, want %q", got, config.ProfileName("astrophysicist"))
	}

	data, err := os.ReadFile(filepath.Join(workDir, "gud.json"))
	if err != nil {
		t.Fatalf("read gud.json: %v", err)
	}

	if !strings.Contains(string(data), "astrophysicist") {
		t.Errorf("gud.json missing profile slug:\n  got: %q", string(data))
	}

	if want := `Persona "astrophysicist" selected.`; !strings.Contains(buf.String(), want) {
		t.Errorf("output missing %q:\n  got: %q", want, buf.String())
	}
}

// TestFetchSuggestions_EmptyRepo verifies the gate in fetchSuggestions: an
// empty (non-repo) directory yields zero total files, so the function returns
// nil, nil before any catalog fetch — no network, no profileManager use.
func TestFetchSuggestions_EmptyRepo(t *testing.T) {
	t.Parallel()

	got, err := fetchSuggestions(t.Context(), t.TempDir())
	if err != nil {
		t.Fatalf("fetchSuggestions(empty dir): %v", err)
	}

	if len(got) != 0 {
		t.Errorf("fetchSuggestions(empty dir) = %+v, want empty", got)
	}
}

// TestIsTerminal verifies the character-device check on both sides: a pipe
// and a regular file are not terminals; /dev/null is, where the platform
// provides it.
func TestIsTerminal(t *testing.T) {
	t.Parallel()

	if isTerminal(os.Stdout) {
		// Not fatal: CI runners vary. Under `go test`, stdout is usually a pipe.
		t.Log("stdout reports as a terminal (unusual for test runs); skipping pipe case")
	} else {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}

		t.Cleanup(func() {
			_ = r.Close()
			_ = w.Close()
		})

		if isTerminal(w) {
			t.Error("isTerminal(pipe) = true, want false")
		}
	}

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("no character device available on this platform")
		}

		t.Fatalf("open %s: %v", os.DevNull, err)
	}

	t.Cleanup(func() { _ = devNull.Close() })

	if !isTerminal(devNull) {
		t.Errorf("isTerminal(%s) = false, want true (character device)", os.DevNull)
	}
}

// TestHasSkipMarker verifies marker detection: false before the marker is
// written, true after writeSkipMarker creates it, and false for a missing
// directory — a stat failure must not read as "suggestion skipped".
func TestHasSkipMarker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	if hasSkipMarker(dir) {
		t.Fatal("hasSkipMarker() = true before the marker is written")
	}

	if hasSkipMarker(filepath.Join(dir, "missing")) {
		t.Fatal("hasSkipMarker() = true for a nonexistent directory")
	}

	writeSkipMarker(dir)

	if !hasSkipMarker(dir) {
		t.Fatal("hasSkipMarker() = false after writeSkipMarker")
	}
}

// TestWriteSkipMarker verifies the marker file: the documented skip text and
// owner read/write permissions. writeSkipMarker is deliberately best-effort
// (errors are swallowed) so a failed write never blocks a commit; that
// contract is not exercised here because it would depend on umask/root.
func TestWriteSkipMarker(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	writeSkipMarker(dir)

	data, err := os.ReadFile(filepath.Join(dir, skipMarker))
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}

	if want := "# gud persona suggestion skipped\n"; string(data) != want {
		t.Errorf("marker content = %q, want %q", string(data), want)
	}

	info, err := os.Stat(filepath.Join(dir, skipMarker))
	if err != nil {
		t.Fatalf("stat marker: %v", err)
	}

	if perm := info.Mode().Perm(); perm&0600 != 0600 {
		t.Errorf("marker permissions = %o, want owner read/write (0600)", perm)
	}
}
