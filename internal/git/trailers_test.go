package git

import (
	"context"
	"testing"
)

func TestAppendTrailers(t *testing.T) {
	tests := []struct {
		name     string
		msg      string
		trailers []Trailer
		want     string
	}{
		{
			name:     "appends after body with blank separator",
			msg:      "feat: add login\n\nImplement JWT auth",
			trailers: []Trailer{{Key: "Fixes", Value: "#123"}},
			want:     "feat: add login\n\nImplement JWT auth\n\nFixes: #123\n",
		},
		{
			name:     "creates trailer block for subject-only message",
			msg:      "docs: update README",
			trailers: []Trailer{{Key: "Fixes", Value: "#7"}},
			want:     "docs: update README\n\nFixes: #7\n",
		},
		{
			name:     "does not duplicate an existing trailer",
			msg:      "fix: resolve crash\n\nAssisted-by: gemini-flash-latest\n",
			trailers: []Trailer{{Key: "Assisted-by", Value: "gemini-flash-latest"}},
			want:     "fix: resolve crash\n\nAssisted-by: gemini-flash-latest\n",
		},
		{
			name:     "keeps flag order across same-key trailers",
			msg:      "fix: resolve crash",
			trailers: []Trailer{{Key: "Fixes", Value: "#1"}, {Key: "Fixes", Value: "#2"}},
			want:     "fix: resolve crash\n\nFixes: #1\nFixes: #2\n",
		},
		{
			name:     "keeps flag order across different keys",
			msg:      "chore: bump deps",
			trailers: []Trailer{{Key: "Fixes", Value: "#3"}, {Key: "Assisted-by", Value: "gemini-flash-lite-latest"}},
			want:     "chore: bump deps\n\nFixes: #3\nAssisted-by: gemini-flash-lite-latest\n",
		},
		{
			name:     "appends after an existing trailer block",
			msg:      "fix: resolve crash\n\nAssisted-by: gemini-flash-latest\n",
			trailers: []Trailer{{Key: "Fixes", Value: "#123"}},
			want:     "fix: resolve crash\n\nAssisted-by: gemini-flash-latest\nFixes: #123\n",
		},
		{
			name: "no trailers leaves message unchanged",
			msg:  "docs: update README",
			want: "docs: update README",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AppendTrailers(context.Background(), tt.msg, tt.trailers)
			if err != nil {
				t.Fatalf("AppendTrailers: %v", err)
			}

			if got != tt.want {
				t.Errorf("AppendTrailers:\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}
