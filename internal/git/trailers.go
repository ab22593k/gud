package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdin = bytes.NewBufferString(message)

	var out bytes.Buffer

	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git interpret-trailers: %w", err)
	}

	return out.String(), nil
}
