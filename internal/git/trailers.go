package git

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// Trailer is a single git commit-message trailer (e.g. "Fixes: #123").
type Trailer struct {
	Key   string
	Value string
}

// trailerKeyRe constrains trailer keys to letters and hyphens. Keys are
// concatenated into --trailer arguments, so an unvalidated key carrying a
// newline would forge extra trailer lines in the message. Today's callers
// only use Fixes/Assisted-by, but any extension (e.g. user-configured keys)
// must pass this gate.
var trailerKeyRe = regexp.MustCompile(`^[A-Za-z-]+$`)

// AppendTrailers pipes message through git interpret-trailers so the given
// trailers are parsed and placed by git itself: an existing trailer block is
// normalised, trailers whose key and value already exist are skipped (git's
// default addIfDifferent rule), and new trailers are appended at the end of
// the block in the given order. Messages without a body get a trailer block
// created with a blank separator. With no trailers to add, message is
// returned unchanged.
//
// git's own parser is the single source of truth for trailer formatting, so a
// failure here means git rejected the arguments rather than that gud mislaid
// the trailers; the error surfaces unchanged.
//
// Keys are validated up front against ^[A-Za-z-]+$: anything else (empty,
// spaces, newlines, punctuation) is rejected before reaching git, so a future
// caller cannot smuggle extra trailer lines through the key. A leading dash
// is harmless — key and value travel as one argv element after --trailer, so
// it can never be parsed as a flag — but stays expressible for keys like
// "Assisted-by".
func AppendTrailers(ctx context.Context, message string, trailers []Trailer) (string, error) {
	if len(trailers) == 0 {
		return message, nil
	}

	// addIfDifferent deduplicates a key=value pair anywhere in the block
	// (git's default addIfDifferentNeighbor only checks adjacency).
	args := []string{"interpret-trailers", "--if-exists", "addIfDifferent"}
	for _, tr := range trailers {
		if !trailerKeyRe.MatchString(tr.Key) {
			return "", fmt.Errorf("invalid trailer key %q: must match ^[A-Za-z-]+$", tr.Key)
		}

		if strings.ContainsAny(tr.Value, "\r\n") {
			return "", fmt.Errorf("invalid trailer value for %q: must not contain CR or LF", tr.Key)
		}

		args = append(args, "--trailer", tr.Key+": "+tr.Value)
	}

	return runGitStdin(ctx, message, args...)
}
