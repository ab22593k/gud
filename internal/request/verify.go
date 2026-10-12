package request

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/clipperhouse/displaywidth"
	"gud/internal/git"
)

// Finding is one failed output-quality check on a generated message.
type Finding struct {
	// Check is the machine-readable code: empty, subject-length,
	// fence-remnant, or file-grounding.
	Check string
	// Detail is the human-readable explanation.
	Detail string
}

// pathToken matches file-like tokens such as main.go or pkg/cache/lru.go.
var pathToken = regexp.MustCompile(`[\w][\w./-]*\.\w+`)

// VerifyMessage checks a generated commit message against its diff and
// returns one Finding per failed check (nil when clean). It is pure:
// no I/O, no model calls, safe to run on every generation.
func VerifyMessage(msg, diff string, wrapLine int) []Finding {
	if strings.TrimSpace(msg) == "" {
		return []Finding{{Check: "empty", Detail: "message is empty"}}
	}

	var out []Finding

	limit := wrapLine
	if limit <= 0 {
		limit = defaultWrapLine
	}

	if subject, _, _ := strings.Cut(msg, "\n"); displaywidth.String(subject) > limit {
		out = append(out, Finding{
			Check:  "subject-length",
			Detail: fmt.Sprintf("subject is %d cols, limit is %d", displaywidth.String(subject), limit),
		})
	}

	if strings.Contains(msg, "```") {
		out = append(out, Finding{Check: "fence-remnant", Detail: "message contains ``` fence"})
	} else if strings.Contains(msg, "`") {
		out = append(out, Finding{Check: "fence-remnant", Detail: "message contains backtick"})
	}

	return append(out, groundingFindings(msg, diff)...)
}

// groundingFindings reports one Finding per path-like token in msg that
// names no file in the diff. A message without tokens skips the diff scan
// entirely: with nothing to ground, no finding is possible, so scanning the
// whole diff would be pure waste.
//
// The diff is tokenized once into a set of changed paths, diff tokens, and
// their basenames, so each message token needs one map lookup instead of a
// full substring scan (O(n·m) on a 256KiB diff). Basenames keep bare mentions
// ("fix a.txt" for diff path "sub/a.txt") grounded; matches are exact tokens,
// so a mention of "main.go" is no longer grounded by an unrelated
// "domain.go" merely containing it as a substring.
func groundingFindings(msg, diff string) []Finding {
	if !pathToken.MatchString(msg) {
		return nil
	}

	present := map[string]bool{}
	for _, p := range git.ExtractChangedPaths(diff) {
		present[p] = true
		present[path.Base(p)] = true
	}

	indexDiffTokens(present, diff)

	seen := map[string]bool{}

	var ungrounded []string

	for _, tok := range pathToken.FindAllString(msg, -1) {
		if seen[tok] {
			continue
		}

		seen[tok] = true

		if !present[tok] {
			ungrounded = append(ungrounded, tok)
		}
	}

	sort.Strings(ungrounded)

	out := make([]Finding, 0, len(ungrounded))

	for _, tok := range ungrounded {
		out = append(out, Finding{Check: "file-grounding", Detail: "mentions " + tok + ", not present in diff"})
	}

	return out
}

// findingCodes returns the check codes of findings for compact logging.
func findingCodes(fs []Finding) []string {
	codes := make([]string, 0, len(fs))
	for _, f := range fs {
		codes = append(codes, f.Check)
	}
	return codes
}

// isTokenWord reports whether c can start a path token: ASCII word
// characters, matching pathToken's leading [\w].
func isTokenWord(c byte) bool {
	return c == '_' || ('0' <= c && c <= '9') || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

// isTokenChar reports whether c can continue a path token, matching
// pathToken's [\w./-] class.
func isTokenChar(c byte) bool {
	return isTokenWord(c) || c == '.' || c == '/' || c == '-'
}

// indexDiffTokens adds every path-like token in diff to set, plus each
// token's basename, in one linear pass. It finds the same tokens as
// pathToken.FindAllString (see TestIndexDiffTokensMatchesPathToken) without
// running the regex over the whole diff: that pattern's greedy
// [\w./-]*\. prefix scans long dotless runs (minified lines, blobs) at ~4MB/s,
// dwarfing the substring scans it was meant to replace.
func indexDiffTokens(set map[string]bool, diff string) {
	n := len(diff)
	i := 0

	for i < n {
		if !isTokenWord(diff[i]) {
			i++

			continue
		}

		// Maximal token-char run from i.
		j := i + 1
		for j < n && isTokenChar(diff[j]) {
			j++
		}

		// The greedy match anchors at the last dot in [i, j) followed by a
		// word char; with none, no token can start anywhere in this run, so
		// skip it whole instead of retrying per position.
		e := -1

		for k := j - 1; k > i; k-- {
			if diff[k] == '.' && k+1 < j && isTokenWord(diff[k+1]) {
				e = k

				break
			}
		}

		if e < 0 {
			i = j

			continue
		}

		k := e + 1
		for k < j && isTokenWord(diff[k]) {
			k++
		}

		tok := diff[i:k]
		set[tok] = true
		set[path.Base(tok)] = true
		i = k
	}
}
