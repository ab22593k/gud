package request

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"gud/internal/git"
	"gud/internal/textwrap"
)

// Finding is one failed output-quality check on a generated message.
type Finding struct {
	// Check is the machine-readable code: empty, empty-body,
	// subject-length, fence-remnant, or file-grounding.
	Check string
	// Detail is the human-readable explanation.
	Detail string
}

// pathToken matches file-like tokens such as main.go or pkg/cache/lru.go.
var pathToken = regexp.MustCompile(`[\w][\w./-]*\.\w+`)

// Change is the evidence a generated message is verified against: the
// diff patch plus the repo-relative paths git reports for the same change
// (both sides of every rename and copy, no a/ or b/ prefix). Paths == nil
// means the probe was unavailable, and verification falls back to
// diff-derived paths alone.
type Change struct {
	Diff  string
	Paths []string
}

// VerifyMessage checks a generated commit message against its diff and
// returns one Finding per failed check (nil when clean). It is pure:
// no I/O, no model calls, safe to run on every generation.
func VerifyMessage(msg string, change Change, wrapLine int) []Finding {
	if strings.TrimSpace(msg) == "" {
		return []Finding{{Check: "empty", Detail: "message is empty"}}
	}

	var out []Finding

	// A subject-only reply reads as a label, not documentation: FR-001
	// requires a body after the subject line and its blank separator.
	if !hasBody(msg) {
		out = append(out, Finding{
			Check:  "empty-body",
			Detail: "message has no body after the subject line",
		})
	}

	limit := wrapLine
	if limit <= 0 {
		limit = defaultWrapLine
	}

	// One measure (D7): textwrap.Columns is the same cell measure the
	// finalization wrapper uses, evaluated once and reused in the detail.
	subject, _, _ := strings.Cut(msg, "\n")
	if cols := textwrap.Columns(subject); cols > limit {
		out = append(out, Finding{
			Check:  "subject-length",
			Detail: fmt.Sprintf("subject is %d cols, limit is %d", cols, limit),
		})
	}

	if strings.Contains(msg, "```") {
		out = append(out, Finding{Check: "fence-remnant", Detail: "message contains ``` fence"})
	} else if strings.Contains(msg, "`") {
		out = append(out, Finding{Check: "fence-remnant", Detail: "message contains backtick"})
	}

	return append(out, groundingFindings(msg, change)...)
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
func groundingFindings(msg string, change Change) []Finding {
	if !pathToken.MatchString(msg) {
		return nil
	}

	present := make(map[string]bool)
	for _, p := range change.Paths {
		present[p] = true
		present[path.Base(p)] = true
	}

	for _, p := range git.ExtractChangedPaths(change.Diff) {
		present[p] = true
		present[path.Base(p)] = true
	}

	indexDiffTokens(present, change.Diff)

	seen := make(map[string]bool)

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

// hasBody reports whether msg carries a non-empty body per FR-001: the
// subject line, then a blank separator line, then at least one non-blank
// line. It scans only the (small) message, never the diff, so the check
// stays within the single-pass budget of FR-019.
func hasBody(msg string) bool {
	lines := strings.Split(msg, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[1]) != "" {
		return false
	}

	for _, line := range lines[2:] {
		if strings.TrimSpace(line) != "" {
			return true
		}
	}

	return false
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
