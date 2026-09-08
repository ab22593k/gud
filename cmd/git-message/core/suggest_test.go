package core

import (
	"bytes"
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
// in gud.json in the working directory.
//
// Not parallel: applySelectedProfile resolves gud.json's location from the
// process CWD, so the test must chdir exclusively.
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
	t.Chdir(workDir)

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

	if want := `Profile "astrophysicist" selected.`; !strings.Contains(buf.String(), want) {
		t.Errorf("output missing %q:\n  got: %q", want, buf.String())
	}
}
