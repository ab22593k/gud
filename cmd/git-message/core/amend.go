package core

import (
	"github.com/spf13/cobra"
)

// amendTarget reads the --amend flag and positional args. The second return
// reports whether amend mode was requested. Forms supported:
// --amend (bare) → HEAD; --amend HEAD~2 (space) → HEAD~2 via positional;
// --amend=HEAD~2 → HEAD~2 (explicit value always wins, except an explicit
// =HEAD combined with a positional, which resolves the positional — the
// review loop always prints the resolved SHA before acting).
func amendTarget(cmd *cobra.Command, args []string) (string, bool) {
	flags := cmd.Flags()
	if !flags.Changed("amend") {
		return "", false
	}
	rev, err := flags.GetString("amend")
	if err != nil {
		return "HEAD", true
	}
	if rev == "" || (rev == "HEAD" && len(args) > 0) {
		if len(args) > 0 {
			return args[0], true
		}
		return "HEAD", true
	}
	return rev, true
}
