package detect

import (
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// gitignorePattern is a single compiled pattern from a .gitignore file.
// re is nil when the pattern could not be compiled (it is then ignored).
type gitignorePattern struct {
	re       *regexp.Regexp
	negate   bool
	rootOnly bool // pattern contains a slash: relative to the .gitignore dir
}

// gitignoreMatcher prunes the stats walk using the repo's root .gitignore,
// so vendored/generated trees (node_modules, vendor, build output) do not
// dominate walk time or pollute the extension statistics.
type gitignoreMatcher struct {
	patterns []gitignorePattern
}

// loadGitignore reads repoRoot/.gitignore. A missing or unreadable file
// yields an empty matcher: the walk then skips only .git.
func loadGitignore(repoRoot string) *gitignoreMatcher {
	//nolint:gosec // G304: repoRoot is the user's own repo directory from the CLI.
	data, err := os.ReadFile(filepath.Join(repoRoot, ".gitignore"))
	if err != nil {
		return &gitignoreMatcher{}
	}

	return &gitignoreMatcher{patterns: parseGitignore(string(data))}
}

// parseGitignore compiles .gitignore patterns. Blank lines and '#' comments
// are ignored; '!' negates a pattern; a trailing '/' marks a directory-only
// pattern (the walk only prunes directories, so it is dropped); a leading or
// embedded '/' anchors the pattern to the .gitignore directory (repo root),
// while patterns without a slash match at any depth. Glob syntax: *, ?,
// [...], and ** (which crosses directory boundaries). The last matching
// pattern wins, mirroring git.
func parseGitignore(data string) []gitignorePattern {
	var patterns []gitignorePattern

	for line := range strings.SplitSeq(data, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		negate := false
		if strings.HasPrefix(line, "!") {
			negate = true
			line = line[1:]
		}

		line = strings.TrimSuffix(line, "/")
		if line == "" {
			continue
		}

		rootOnly := false
		if strings.HasPrefix(line, "/") {
			rootOnly = true
			line = line[1:]
		}

		if strings.Contains(line, "/") {
			rootOnly = true
		}

		if line == "" {
			continue
		}

		re, err := regexp.Compile(globToRegexp(line))
		if err != nil {
			continue // drop malformed patterns silently
		}

		patterns = append(patterns, gitignorePattern{re: re, negate: negate, rootOnly: rootOnly})
	}

	return patterns
}

// ignored reports whether the directory at rel (slash-separated path relative
// to the repo root) should be pruned. The last matching pattern wins.
func (m *gitignoreMatcher) ignored(rel string) bool {
	ignored := false
	base := path.Base(rel)

	for _, p := range m.patterns {
		if p.re == nil {
			continue
		}

		var matched bool
		if p.rootOnly {
			matched = p.re.MatchString(rel)
		} else {
			matched = p.re.MatchString(base)
		}

		if matched {
			ignored = !p.negate
		}
	}

	return ignored
}

// globToRegexp translates a gitignore glob into an anchored regular
// expression. '*' and '?' do not cross '/'; '**' crosses any number of
// directories (a leading '**/' also matches zero directories). All other
// regexp metacharacters are escaped so user patterns cannot inject syntax.
func globToRegexp(pattern string) string {
	var sb strings.Builder
	sb.WriteString("^")

	// Leading "**/" matches zero or more directories, so strip it and let
	// the pattern match from any ancestor depth.
	if strings.HasPrefix(pattern, "**/") {
		sb.WriteString("(?:.*/)?")

		pattern = pattern[3:]
	}

	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				sb.WriteString(".*")

				i++
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		case '[':
			// Copy the character class through its closing ']'. Git uses '!'
			// for negation where RE2 uses '^', so translate it.
			j := i + 1
			if j < len(pattern) && pattern[j] == '!' {
				j++
			}

			if j < len(pattern) && pattern[j] == ']' {
				j++
			}

			for j < len(pattern) && pattern[j] != ']' {
				j++
			}

			if j >= len(pattern) {
				sb.WriteString(`\[`)
			} else {
				class := pattern[i : j+1]
				if strings.HasPrefix(class, "[!") {
					class = "^" + class[2:]
				}

				sb.WriteString(class)

				i = j
			}
		default:
			if strings.ContainsRune(`.+()|{}^$\\`, rune(c)) {
				sb.WriteByte('\\')
			}

			sb.WriteByte(c)
		}
	}

	sb.WriteString("$")

	return sb.String()
}

// noExtensionKey marks files without an extension in FilesByExtension. It is
// plumbing for counts, never a keyword for profile suggestion.
const noExtensionKey = "(no extension)"

// MaxFilesForStats caps files counted by ComputeStatsWithContext so a huge
// monorepo cannot blow memory or prompt size.
const MaxFilesForStats = 20000
