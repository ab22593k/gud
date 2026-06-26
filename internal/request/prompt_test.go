package request

import (
	"strings"
	"testing"
)

func TestBuildCommitMessagePrompt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		diff        string
		context     string
		detailLevel DetailLevel
		hint        string
		profile     ProfileName
		validate    func(t *testing.T, prompt string)
	}{
		{
			name:        "basic diff creates prompt with diff",
			diff:        "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "fmt.Println") {
					t.Errorf("prompt should contain diff content")
				}
				if !strings.Contains(prompt, "Diff:") {
					t.Errorf("prompt should mention Diff:")
				}
			},
		},
		{
			name:        "with context adds context to prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "Adding hello world feature",
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
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
			detailLevel: DetailMinimal,
			hint:        "",
			profile:     "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "72") && !strings.Contains(prompt, "Concise") {
					t.Errorf("minimal prompt should mention concise")
				}
			},
		},
		{
			name:        "detailed detail level produces comprehensive prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailDetailed,
			hint:        "",
			profile:     "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "72") && !strings.Contains(prompt, "82") {
					t.Errorf("detailed prompt should mention character limits")
				}
			},
		},
		{
			name:        "hint adds focus boundaries to prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "focus on security changes",
			profile:     "",
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
			detailLevel: DetailStandard,
			hint:        "",
			profile:     "",
			validate: func(t *testing.T, prompt string) {
				if prompt == "" {
					t.Errorf("prompt should not be empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prompt := BuildCommitMessagePrompt(tt.diff, tt.context, tt.detailLevel, tt.hint, tt.profile)
			tt.validate(t, prompt)
		})
	}
}

func TestBuildCommitMessagePromptWithEmptyProfile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		diff        string
		context     string
		detailLevel DetailLevel
		hint        string
		validate    func(t *testing.T, prompt string)
	}{
		{
			name:        "diff appears in prompt",
			diff:        "diff --git a/main.go b/main.go\n+func add(a, b int) int { return a + b }",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "func add") {
					t.Errorf("prompt should contain diff content")
				}
			},
		},
		{
			name:        "detailed level mentions wrap limit",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailDetailed,
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "72") && !strings.Contains(prompt, "82") {
					t.Errorf("detailed prompt should mention character limits")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prompt := BuildCommitMessagePrompt(tt.diff, tt.context, tt.detailLevel, tt.hint, "")
			tt.validate(t, prompt)
		})
	}
}
