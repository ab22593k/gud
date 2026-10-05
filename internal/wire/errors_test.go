package wire

import (
	"errors"
	"fmt"
	"testing"
)

func TestNextActionCoversSentinels(t *testing.T) {
	t.Parallel()

	sentinels := []error{
		ErrBadURL,
		ErrUnknownRef,
		ErrMissingPath,
		ErrTargetNotEmpty,
		ErrNotACheckout,
		ErrInvalidRecord,
		ErrDiverged,
		ErrUpstream,
	}

	for _, s := range sentinels {
		wrapped := fmt.Errorf("update /tmp/x: %w", s)
		if got := NextAction(wrapped); got == "" {
			t.Errorf("NextAction(%v) is empty", s)
		}
	}
}

func TestNextActionUnknownError(t *testing.T) {
	t.Parallel()

	if got := NextAction(errors.New("boom")); got == "" {
		t.Fatal("NextAction(unknown) is empty")
	}
}
