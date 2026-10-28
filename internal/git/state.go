package git

import (
	"context"
	"fmt"
)

// HasCommits reports whether the repository has at least one commit. It is
// the D8 commit-state probe: a single `rev-parse --verify --quiet
// HEAD^{commit}` whose exit code is the answer — 0 means commits exist, 1
// means the branch is unborn (HEAD resolves to nothing). Any other failure
// (non-repository, broken git, permission problem) is returned as an error,
// never as an absence, so an unreadable repository is never reported as a
// repository with no commits.
func HasCommits(ctx context.Context) (bool, error) {
	if _, err := runGit(ctx, "rev-parse", "--verify", "--quiet", "HEAD^{commit}"); err != nil {
		if isGitExitCode(err, 1) {
			return false, nil
		}

		return false, fmt.Errorf("check for commits: %w", err)
	}

	return true, nil
}
