package wire

import (
	"errors"
	"strings"
	"testing"
)

func TestParseSourceURL(t *testing.T) {
	t.Parallel()

	t.Run("oca example", func(t *testing.T) {
		t.Parallel()

		got, err := ParseSourceURL("https://github.com/OCA/server-tools/tree/19.0/auto_backup")
		if err != nil {
			t.Fatalf("ParseSourceURL: %v", err)
		}

		want := SourceRef{
			Host:      "github.com",
			Owner:     "OCA",
			Repo:      "server-tools",
			Ref:       "19.0",
			Subpath:   "auto_backup",
			SourceURL: "https://github.com/OCA/server-tools/tree/19.0/auto_backup",
		}

		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("tolerated forms", func(t *testing.T) {
		t.Parallel()

		got, err := ParseSourceURL("https://github.com/OCA/server-tools.git/tree/19.0/auto_backup/")
		if err != nil {
			t.Fatalf("ParseSourceURL: %v", err)
		}

		if got.Repo != "server-tools" || got.Subpath != "auto_backup" {
			t.Fatalf("unexpected parse: %+v", got)
		}

		nested, err := ParseSourceURL("https://example.com/a/b/tree/main/x/y/z")
		if err != nil {
			t.Fatalf("ParseSourceURL: %v", err)
		}

		if nested.Ref != "main" || nested.Subpath != "x/y/z" {
			t.Fatalf("unexpected parse: %+v", nested)
		}
	})

	t.Run("display", func(t *testing.T) {
		t.Parallel()

		s := SourceRef{Host: "github.com", Owner: "OCA", Repo: "server-tools", Ref: "19.0", Subpath: "auto_backup"}

		if got, want := s.Display(), "github.com/OCA/server-tools@19.0:auto_backup"; got != want {
			t.Fatalf("Display = %q, want %q", got, want)
		}
	})

	rejects := []struct {
		name string
		url  string
	}{
		{name: "empty", url: ""},
		{name: "http scheme", url: "http://github.com/o/r/tree/main/p"},
		{name: "ssh style", url: "git@github.com:o/r.git"},
		{name: "credentials", url: "https://" + "token:x@" + "github.com/o/r/tree/main/p"},
		{name: "port", url: "https://github.com:8443/o/r/tree/main/p"},
		{name: "missing tree", url: "https://github.com/o/r/blob/main/p"},
		{name: "missing subpath", url: "https://github.com/o/r/tree/main"},
		{name: "missing ref", url: "https://github.com/o/r/tree"},
		{name: "dotdot subpath", url: "https://github.com/o/r/tree/main/../evil"},
		{name: "leading dash ref", url: "https://github.com/o/r/tree/-x/p"},
		{name: "leading dash host", url: "https://-evil.com/o/r/tree/main/p"},
		{name: "whitespace", url: "https://github.com/o/r/tree/main/a b"},
	}

	for _, tc := range rejects {
		t.Run("reject "+tc.name, func(t *testing.T) {
			t.Parallel()

			if _, err := ParseSourceURL(tc.url); !errors.Is(err, ErrBadURL) {
				t.Fatalf("ParseSourceURL(%q) err = %v, want ErrBadURL", tc.url, err)
			}
		})
	}
}

func TestIsPinnedSHA(t *testing.T) {
	t.Parallel()

	if !IsPinnedSHA(strings.Repeat("a", 40)) {
		t.Fatal("40-hex should pin")
	}

	for _, ref := range []string{"main", "19.0", strings.Repeat("a", 39), strings.Repeat("z", 40), ""} {
		if IsPinnedSHA(ref) {
			t.Fatalf("ref %q should not pin", ref)
		}
	}
}

func TestResolveRef(t *testing.T) {
	t.Parallel()

	known := []string{"main", "feature/foo", "19.0"}

	cases := []struct {
		name     string
		ref      string
		subpath  string
		wantRef  string
		wantPath string
		wantOK   bool
	}{
		{name: "plain branch", ref: "main", subpath: "p", wantRef: "main", wantPath: "p", wantOK: true},
		{
			name: "slashed branch wins longest", ref: "feature", subpath: "foo/bar",
			wantRef: "feature/foo", wantPath: "bar", wantOK: true,
		},
		{name: "exact tag", ref: "19.0", subpath: "auto_backup", wantRef: "19.0", wantPath: "auto_backup", wantOK: true},
		{name: "unknown", ref: "nope", subpath: "p", wantOK: false},
		{name: "root remainder", ref: "main", subpath: "", wantRef: "main", wantPath: "", wantOK: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gotRef, gotPath, gotOK := ResolveRef(known, tc.ref, tc.subpath)
			if gotRef != tc.wantRef || gotPath != tc.wantPath || gotOK != tc.wantOK {
				t.Fatalf("got (%q,%q,%v), want (%q,%q,%v)", gotRef, gotPath, gotOK, tc.wantRef, tc.wantPath, tc.wantOK)
			}
		})
	}
}

func TestShortSHA(t *testing.T) {
	t.Parallel()

	if got := ShortSHA(strings.Repeat("a", 40)); len(got) != 7 {
		t.Fatalf("ShortSHA len = %d, want 7", len(got))
	}

	if got := ShortSHA("abc"); got != "abc" {
		t.Fatalf("ShortSHA short input = %q", got)
	}
}

func TestSourceURLs(t *testing.T) {
	t.Parallel()

	s := SourceRef{Host: "github.com", Owner: "OCA", Repo: "server-tools", Ref: "19.0", Subpath: "auto_backup"}

	if got, want := s.CloneURL(), "https://github.com/OCA/server-tools"; got != want {
		t.Fatalf("CloneURL = %q, want %q", got, want)
	}

	a := SourceRef{Host: "h", Owner: "o", Repo: "r", Ref: "main"}
	b := SourceRef{Host: "h", Owner: "o", Repo: "r", Ref: "main", Subpath: "x"}

	if a.CacheKey() != b.CacheKey() {
		t.Fatal("same repo+ref should share a cache key across subpaths")
	}

	c := SourceRef{Host: "h", Owner: "o", Repo: "r", Ref: "other"}
	if a.CacheKey() == c.CacheKey() {
		t.Fatal("different refs must not share a cache key")
	}
}
