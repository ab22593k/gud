package wire

import (
	"errors"
	"testing"
)

func TestParseSourceURLGenericRepository(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		raw  string
		host string
		own  string
		repo string
	}{
		{"plain", "https://github.com/owner/repo", "github.com", "owner", "repo"},
		{"dotgit", "https://github.com/owner/repo.git", "github.com", "owner", "repo"},
		{"trailing slash", "https://github.com/owner/repo/", "github.com", "owner", "repo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseSourceURL(tc.raw)
			if err != nil {
				t.Fatalf("ParseSourceURL(%q) = %v", tc.raw, err)
			}

			if got.Host != tc.host || got.Owner != tc.own || got.Repo != tc.repo {
				t.Errorf("identity = %s/%s/%s, want %s/%s/%s",
					got.Host, got.Owner, got.Repo, tc.host, tc.own, tc.repo)
			}

			if got.Ref != "HEAD" {
				t.Errorf("Ref = %q, want HEAD", got.Ref)
			}

			if got.Subpath != "" {
				t.Errorf("Subpath = %q, want empty", got.Subpath)
			}
		})
	}
}

func TestParseSourceURLRejectsGenericURLExtraSegments(t *testing.T) {
	t.Parallel()

	for _, raw := range []string{
		"https://github.com/owner/repo/tree",
		"https://github.com/owner/repo/blob/main/path",
	} {
		if _, err := ParseSourceURL(raw); !errors.Is(err, ErrBadURL) {
			t.Errorf("ParseSourceURL(%q) err = %v, want ErrBadURL", raw, err)
		}
	}
}

func TestParseSubpath(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		segs    []string
		want    string
		wantErr bool
	}{
		{"single", []string{"auto_backup"}, "auto_backup", false},
		{"nested", []string{"path", "sub"}, "path/sub", false},
		{"empty segment", []string{"path", "", "sub"}, "", true},
		{"dot segment", []string{"path", ".", "sub"}, "", true},
		{"dotdot segment", []string{"path", "..", "sub"}, "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ParseSubpath(tc.segs)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSubpath(%v) = %q, want error", tc.segs, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("ParseSubpath(%v) = %v", tc.segs, err)
			}

			if got != tc.want {
				t.Errorf("ParseSubpath(%v) = %q, want %q", tc.segs, got, tc.want)
			}
		})
	}
}
