package core

import (
	"testing"
)

func TestTruncate(t *testing.T) {
	t.Parallel()

	const (
		hello      = "hello"
		helloWorld = "hello world"
	)

	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{name: "shorter than max returns as-is", s: hello, maxLen: 10, want: hello},
		{name: "equal to max returns as-is", s: hello, maxLen: 5, want: hello},
		{name: "longer than max appends ellipsis", s: helloWorld, maxLen: 5, want: "hello..."},
		{name: "empty string returns empty", s: "", maxLen: 10, want: ""},
		{name: "maxLen of zero with content", s: hello, maxLen: 0, want: "..."},
		{name: "trailing space at boundary trimmed", s: helloWorld, maxLen: 6, want: "hello..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
