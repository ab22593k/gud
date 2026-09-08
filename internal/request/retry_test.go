package request

import (
	"errors"
	"testing"
)

func TestIsTransientErr_Classifies(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"timeout-like", errors.New("model error: timeout exceeded"), true},
		{"unavailable", errors.New("model error: unavailable"), true},
		{"rate-limited", errors.New("model error code: 429"), true},
		{"server-error", errors.New("model error code: 500"), true},
		{"invalid-key", errors.New("model error code: 401"), false},
		{"forbidden", errors.New("model error code: 403"), false},
		{"empty", errors.New("generated message is empty"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := isTransientErr(tc.err); got != tc.want {
				t.Errorf("isTransientErr(%v)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
