package git

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

// SubmoduleChange describes a staged gitlink (submodule pointer) change parsed
// from a diff. OldCommit and NewCommit are the commit hashes, preferring the
// full hashes from the "Subproject commit" lines when present; either side is
// the all-zero hash for an added or removed gitlink.
type SubmoduleChange struct {
	Path      string
	OldCommit string
	NewCommit string
}

// gitlinkMode is the file mode of a gitlink (submodule pointer) entry in a
// diff. A raw pointer change shows the model nothing but two opaque hashes, so
// these entries get dedicated context enrichment.
const gitlinkMode = "160000"

// maxSubmoduleLogEntries caps the commit subjects included per submodule so a
// large range cannot blow up the prompt.
const maxSubmoduleLogEntries = MaxRecentCommits

// ExtractSubmoduleChanges parses a diff and returns the gitlink (mode 160000)
// changes it contains. It is pure text parsing: it never touches the
// filesystem, the submodule worktrees, or the network.
func ExtractSubmoduleChanges(diff string) []SubmoduleChange {
	var changes []SubmoduleChange
	for _, entry := range splitDiffEntries(diff) {
		if change, ok := parseGitlinkEntry(entry); ok {
			changes = append(changes, change)
		}
	}
	return changes
}

// parseGitlinkEntry extracts a single gitlink change from a diff entry. The
// "index <old>..<new> 160000" line carries the mode, so it is the reliable
// signal; the "Subproject commit" lines, when present, upgrade the abbreviated
// index hashes to full hashes.
func parseGitlinkEntry(entry string) (SubmoduleChange, bool) {
	path := extractFilePath(entry)
	if path == "" {
		return SubmoduleChange{}, false
	}

	var old, new string
	gitlink := false
	for line := range strings.SplitSeq(entry, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "index "):
			if oldIdx, newIdx, mode, ok := parseIndexLine(trimmed); ok {
				old, new = oldIdx, newIdx
				if mode == gitlinkMode {
					gitlink = true
				}
			}
		case strings.HasPrefix(trimmed, "new file mode "), strings.HasPrefix(trimmed, "deleted file mode "):
			if fields := strings.Fields(trimmed); len(fields) == 4 && fields[3] == gitlinkMode {
				gitlink = true
			}
		default:
			if sha, marker, ok := parseSubprojectLine(trimmed); ok {
				if marker == '-' {
					old = sha
				} else {
					new = sha
				}
			}
		}
	}

	if !gitlink {
		return SubmoduleChange{}, false
	}
	return SubmoduleChange{Path: path, OldCommit: old, NewCommit: new}, true
}

// parseIndexLine extracts the old and new hashes from an "index <old>..<new>"
// diff line, optionally suffixed with a mode. Add/remove index lines carry no
// mode (it lives on the "new/deleted file mode" line), so mode is "" there;
// a present non-gitlink mode marks the entry as a regular file.
func parseIndexLine(line string) (old, new, mode string, ok bool) {
	rest, found := strings.CutPrefix(line, "index ")
	if !found {
		return "", "", "", false
	}
	fields := strings.Fields(rest)
	if len(fields) < 1 || len(fields) > 2 {
		return "", "", "", false
	}
	old, new, ok = strings.Cut(fields[0], "..")
	if !ok {
		return "", "", "", false
	}
	if len(fields) == 2 {
		mode = fields[1]
	}
	return old, new, mode, true
}

// parseSubprojectLine extracts the commit hash from a diff content line of the
// form "-Subproject commit <sha>" (removed) or "+Subproject commit <sha>"
// (added), reporting the diff marker so the caller can place the hash.
func parseSubprojectLine(line string) (sha string, marker byte, ok bool) {
	if line == "" {
		return "", 0, false
	}
	marker = line[0]
	if marker != '-' && marker != '+' && marker != ' ' {
		return "", 0, false
	}
	rest := strings.TrimSpace(line[1:])
	if !strings.HasPrefix(rest, "Subproject commit ") {
		return "", 0, false
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, "Subproject commit ")), marker, true
}

// SubmoduleContext builds a prompt-context fragment describing staged gitlink
// changes: each submodule's name, path, URL, the old→new commit range, and —
// when the submodule is checked out with locally available history — the
// subjects of the commits in that range. It is best-effort and local-only: no
// network access, and any failure (missing checkout, absent .gitmodules)
// degrades to a SHA-only summary. Returns "" when there are no changes.
func SubmoduleContext(ctx context.Context, root string, changes []SubmoduleChange) string {
	if len(changes) == 0 {
		return ""
	}

	names, urls := readGitmodules(ctx, root)

	var b strings.Builder
	b.WriteString("Submodule (gitlink) changes:\n")
	for _, ch := range changes {
		name := names[ch.Path]
		if name == "" {
			name = ch.Path
		}
		b.WriteString("- ")
		b.WriteString(name)
		if url := urls[ch.Path]; url != "" {
			b.WriteString(" (")
			b.WriteString(url)
			b.WriteString(")")
		}
		b.WriteString(": ")
		b.WriteString(shortRange(ch))
		for _, subject := range submoduleSubjects(ctx, root, ch) {
			b.WriteString("\n  > ")
			b.WriteString(subject)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// shortRange renders the old→new commit range for a gitlink change, noting
// adds and removals (all-zero side).
func shortRange(ch SubmoduleChange) string {
	switch {
	case isZeroSHA(ch.OldCommit):
		return "added at " + shortSHA(ch.NewCommit)
	case isZeroSHA(ch.NewCommit):
		return "removed from " + shortSHA(ch.OldCommit)
	default:
		return shortSHA(ch.OldCommit) + ".." + shortSHA(ch.NewCommit)
	}
}

// shortSHA abbreviates a commit hash to git's default 7-character form,
// leaving already-short hashes untouched.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// isZeroSHA reports whether sha is the all-zero hash (a gitlink add or remove).
func isZeroSHA(sha string) bool {
	return strings.Trim(sha, "0") == ""
}

// submoduleSubjects returns the subjects of the commits in the change's range,
// resolved from locally available submodule history (no network access).
// Returns nil when the submodule is not checked out, the range is empty or
// one-sided in a way that cannot be logged, or the log cannot be read.
func submoduleSubjects(ctx context.Context, root string, ch SubmoduleChange) []string {
	if ch.OldCommit == ch.NewCommit || isZeroSHA(ch.NewCommit) {
		return nil
	}

	dir := filepath.Join(root, ch.Path)
	var args []string
	if isZeroSHA(ch.OldCommit) {
		// Added gitlink: log the new commit's own history.
		args = []string{"-C", dir, "log", "--oneline", "-n", fmt.Sprint(maxSubmoduleLogEntries), ch.NewCommit}
	} else {
		args = []string{"-C", dir, "log", "--oneline", "-n", fmt.Sprint(maxSubmoduleLogEntries), ch.OldCommit + ".." + ch.NewCommit}
	}

	out := runGitOutput(ctx, args...)
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// readGitmodules returns maps from submodule path → name and path → url,
// parsed from the repo's .gitmodules file via git config. The file is read by
// absolute path so the caller's working directory does not matter. Empty maps
// when the file is absent or unreadable.
func readGitmodules(ctx context.Context, root string) (names, urls map[string]string) {
	namePaths := make(map[string]string)
	nameURLs := make(map[string]string)

	out := runGitOutput(ctx, "config", "-f", filepath.Join(root, ".gitmodules"), "--get-regexp", `^submodule\..*\.`)
	for line := range strings.SplitSeq(out, "\n") {
		key, value, ok := strings.Cut(line, " ")
		if !ok {
			continue
		}
		rest, found := strings.CutPrefix(key, "submodule.")
		if !found {
			continue
		}
		name, prop, found := strings.Cut(rest, ".")
		if !found {
			continue
		}
		switch prop {
		case "path":
			namePaths[name] = value
		case "url":
			nameURLs[name] = value
		}
	}

	names = make(map[string]string)
	urls = make(map[string]string)
	for name, path := range namePaths {
		names[path] = name
		urls[path] = nameURLs[name]
	}
	return names, urls
}
