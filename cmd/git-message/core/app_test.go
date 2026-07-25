package core

import (
	"testing"
)

func TestExtractPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "standard helixdb port", url: "http://localhost:6969", want: "6969"},
		{name: "empty URL returns default", url: "", want: "6969"},
		{name: "non-standard port preserved", url: "http://localhost:8080", want: "8080"},
		{name: "invalid URL returns default", url: "://invalid", want: "6969"},
		{name: "URL without port returns default", url: "http://localhost", want: "6969"},
		{name: "URL with path only", url: "http://localhost/path", want: "6969"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := extractPort(tt.url); got != tt.want {
				t.Errorf("extractPort(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}
