package core

import (
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

const (
	// defaultVersion is the fallback when neither an -X injected version nor
	// module build info provides one (e.g. `go run` from a checkout whose
	// build embeds no VCS info).
	defaultVersion = "0.3.2"

	versionCmdName = "version"
)

// version is overridden at release build time via:
//
//	go build -ldflags "-X gud/cmd/git-message/core.version=v1.2.3" ./cmd/git-message
//
// It must stay a var (not const): the linker rejects -X against constants.
var version string

// effectiveVersion resolves the version string to report: an explicitly
// injected -X value wins; otherwise the main module version from the build
// info (populated for binaries built via `go install pkg@version`);
// otherwise defaultVersion. A "+dirty" suffix marks builds from a tree with
// uncommitted changes, per the vcs.modified build setting.
func effectiveVersion() string {
	v := version
	if v == "" {
		v = moduleVersion()
	}

	if v == "" {
		v = defaultVersion
	}

	if buildDirty() {
		v += "+dirty"
	}

	return v
}

// moduleVersion returns the main module version from the binary's build info,
// or "" when it carries no release signal (see pickModuleVersion).
func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}

	return pickModuleVersion(info.Main.Version)
}

// pickModuleVersion normalises a main module version into a release signal.
// "(devel)" means built from a checkout without module version info, and a
// v0.0.0- base means the checkout has no release tags (Go fills in a
// timestamped pseudo-version): neither identifies a release, so both yield
// "". A toolchain "+dirty" suffix is trimmed — effectiveVersion applies its
// own single marker — and anything else (e.g. v1.2.3 from
// `go install pkg@version`) passes through.
func pickModuleVersion(v string) string {
	v = strings.TrimSuffix(v, "+dirty")
	if v == "" || v == "(devel)" || strings.HasPrefix(v, "v0.0.0-") {
		return ""
	}

	return v
}

// buildDirty reports whether the binary was built from a tree with
// uncommitted changes, per the vcs.modified build setting. Binaries built
// without VCS info carry no settings, which reads as clean.
func buildDirty() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}

	for _, s := range info.Settings {
		if s.Key == "vcs.modified" {
			return s.Value == "true"
		}
	}

	return false
}

var versionCmd = &cobra.Command{
	Use:   versionCmdName,
	Short: "Print the version number",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.Printf("gud version %s\n", effectiveVersion())

		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
