package core

import (
	"testing"
)

func TestHasMeaningfulContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "empty string", text: "", want: false},
		{name: "only blank lines", text: "\n\n  \n", want: false},
		{name: "only git comments", text: "# Please enter commit message\n# Lines starting with # are comments", want: false},
		{name: "single real line", text: "feat: add login", want: true},
		{name: "real line with leading text", text: "  feat: add login\n", want: true},
		{name: "comments then content", text: "# Please enter commit message\nfeat: add login\n# more comments", want: true},
		{name: "content then comments", text: "fix: resolve crash\n# Co-authored-by: someone@example.com", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasMeaningfulContent(tt.text)
			if got != tt.want {
				t.Errorf("hasMeaningfulContent(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
