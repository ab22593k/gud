package git

import (
	"context"
)

// Trailer is a single git commit-message trailer (e.g. "Fixes: #123").
type Trailer struct {
	Key   string
	Value string
}

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
func AppendTrailers(ctx context.Context, message string, trailers []Trailer) (string, error) {
	if len(trailers) == 0 {
		return message, nil
	}

	// addIfDifferent deduplicates a key=value pair anywhere in the block
	// (git's default addIfDifferentNeighbor only checks adjacency).
	args := []string{"interpret-trailers", "--if-exists", "addIfDifferent"}
	for _, tr := range trailers {
		args = append(args, "--trailer", tr.Key+": "+tr.Value)
	}

	return runGitStdin(ctx, message, args...)
}
