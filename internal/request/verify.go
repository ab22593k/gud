package request

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

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

	if subject, _, _ := strings.Cut(msg, "\n"); len(subject) > limit {
		out = append(out, Finding{
			Check:  "subject-length",
			Detail: fmt.Sprintf("subject is %d chars, limit is %d", len(subject), limit),
		})
	}

	if strings.Contains(msg, "```") {
		out = append(out, Finding{Check: "fence-remnant", Detail: "message contains ``` fence"})
	} else if strings.Contains(msg, "`") {
		out = append(out, Finding{Check: "fence-remnant", Detail: "message contains backtick"})
	}

	allowed := map[string]bool{}
	for _, p := range git.ExtractChangedPaths(diff) {
		allowed[p] = true
	}

	seen := map[string]bool{}
	var ungrounded []string

	for _, tok := range pathToken.FindAllString(msg, -1) {
		if seen[tok] {
			continue
		}

		seen[tok] = true

		if !allowed[tok] && !strings.Contains(diff, tok) {
			ungrounded = append(ungrounded, tok)
		}
	}

	sort.Strings(ungrounded)

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
