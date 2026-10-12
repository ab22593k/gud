package core

import (
	"strings"
	"testing"
)

// TestEffectiveVersionInjectedWins pins -X precedence: an explicitly injected
// version is reported verbatim (plus an optional +dirty suffix), never
// replaced by build info or the default.
func TestEffectiveVersionInjectedWins(t *testing.T) {
	old := version
	version = "v9.9.9-test"

	t.Cleanup(func() { version = old })

	got := effectiveVersion()
	if !strings.HasPrefix(got, "v9.9.9-test") {
		t.Errorf("effectiveVersion() = %q, want prefix %q", got, "v9.9.9-test")
	}
}

// TestEffectiveVersionFallsBack pins the fallback chain: with no -X value,
// the reported version is never empty and stays semver-shaped (module
// version for installed binaries, defaultVersion otherwise).
func TestEffectiveVersionFallsBack(t *testing.T) {
	old := version
	version = ""

	t.Cleanup(func() { version = old })

	got := strings.TrimSuffix(effectiveVersion(), "+dirty")
	if got == "" {
		t.Fatal("effectiveVersion() is empty, want a fallback version")
	}

	if !strings.Contains(got, ".") {
		t.Errorf("effectiveVersion() = %q, want semver shape (X.Y.Z)", got)
	}
}

// TestPickModuleVersion pins which build-info versions count as a release:
// tagged versions pass through (minus any toolchain +dirty suffix), while
// checkout pseudo-versions carry no release signal.
func TestPickModuleVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "devel checkout", input: "(devel)", want: ""},
		{name: "untagged pseudo-version", input: "v0.0.0-20261012102907-3244d83c1d30", want: ""},
		{name: "dirty pseudo-version", input: "v0.0.0-20261012102907-3244d83c1d30+dirty", want: ""},
		{name: "release", input: "v1.2.3", want: "v1.2.3"},
		{name: "release dirty suffix trimmed", input: "v1.2.3+dirty", want: "v1.2.3"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := pickModuleVersion(tc.input); got != tc.want {
				t.Errorf("pickModuleVersion(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
