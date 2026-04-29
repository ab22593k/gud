package request

import (
	"strings"
	"testing"
)

func TestBuildCommitMessagePrompt(t *testing.T) {
	tests := []struct {
		name        string
		diff        string
		context     string
		detailLevel string
		hint        string
		validate    func(t *testing.T, prompt string)
	}{
		{
			name:        "basic diff creates prompt with diff",
			diff:        "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")",
			context:     "",
			detailLevel: "standard",
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "fmt.Println") {
					t.Errorf("prompt should contain diff content")
				}
				if !strings.Contains(prompt, "commit message") {
					t.Errorf("prompt should mention commit message")
				}
			},
		},
		{
			name:        "with context adds context to prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "Adding hello world feature",
			detailLevel: "standard",
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "Adding hello world feature") {
					t.Errorf("prompt should contain context")
				}
			},
		},
		{
			name:        "minimal detail level produces short prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: "minimal",
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "50 characters") {
					t.Errorf("minimal prompt should mention 50 chars limit")
				}
			},
		},
		{
			name:        "detailed detail level produces comprehensive prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: "detailed",
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "body") {
					t.Errorf("detailed prompt should mention body")
				}
			},
		},
		{
			name:        "hint adds focus boundaries to prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: "standard",
			hint:        "focus on security changes",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "focus on security changes") {
					t.Errorf("prompt should contain hint")
				}
			},
		},
		{
			name:        "empty diff still creates valid prompt",
			diff:        "",
			context:     "",
			detailLevel: "standard",
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if prompt == "" {
					t.Errorf("prompt should not be empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := BuildCommitMessagePrompt(tt.diff, tt.context, tt.detailLevel, tt.hint)
			tt.validate(t, prompt)
		})
	}
}
