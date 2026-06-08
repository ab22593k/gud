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
		persona     PersonaName
		validate    func(t *testing.T, prompt string)
	}{
		{
			name:        "basic diff creates prompt with diff",
			diff:        "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
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
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
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
			persona:     PersonaEmbedded,
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "72") && !strings.Contains(prompt, "Subject") {
					t.Errorf("minimal prompt should mention subject line limit")
				}
			},
		},
		{
			name:        "detailed detail level produces comprehensive prompt",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailDetailed,
			hint:        "",
			persona:     PersonaEmbedded,
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
			persona:     PersonaEmbedded,
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
			persona:     PersonaEmbedded,
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
			prompt := BuildCommitMessagePrompt(tt.diff, tt.context, tt.detailLevel, tt.hint, tt.persona)
			tt.validate(t, prompt)
		})
	}
}

func TestBuildCommitMessagePromptWithPersona(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		diff        string
		context     string
		detailLevel DetailLevel
		hint        string
		persona     PersonaName
		validate    func(t *testing.T, prompt string)
	}{
		{
			name:        "embedded persona uses linux mailing list style",
			diff:        "diff --git a/main.go b/main.go\n+func add(a, b int) int { return a + b }",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaEmbedded,
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "Principal") && !strings.Contains(prompt, "Embedded") {
					t.Errorf("embedded prompt should mention Principal Embedded style")
				}
			},
		},
		{
			name:        "conventional persona includes type definitions",
			diff:        "diff --git a/main.go b/main.go\n+func add(a, b int) int { return a + b }",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     PersonaConventional,
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "feat:") {
					t.Errorf("conventional prompt should mention feat type")
				}
				if !strings.Contains(prompt, "fix:") {
					t.Errorf("conventional prompt should mention fix type")
				}
				if !strings.Contains(prompt, "BREAKING CHANGE") {
					t.Errorf("conventional prompt should mention BREAKING CHANGE")
				}
			},
		},
		{
			name:        "conventional persona with minimal detail level",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailMinimal,
			hint:        "",
			persona:     PersonaConventional,
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "<type>") {
					t.Errorf("conventional minimal prompt should mention subject format")
				}
			},
		},
		{
			name:        "default persona falls back to embedded",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			persona:     "",
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "Principal") && !strings.Contains(prompt, "Embedded") {
					t.Errorf("default prompt should use embedded style")
				}
			},
		},
		{
			name:        "embedded with detailed level includes body",
			diff:        "diff --git a/main.go b/main.go",
			context:     "",
			detailLevel: DetailDetailed,
			hint:        "",
			persona:     PersonaEmbedded,
			validate: func(t *testing.T, prompt string) {
				if !strings.Contains(prompt, "82") && !strings.Contains(prompt, "72") {
					t.Errorf("detailed embedded should mention character limits")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			prompt := BuildCommitMessagePrompt(tt.diff, tt.context, tt.detailLevel, tt.hint, tt.persona)
			tt.validate(t, prompt)
		})
	}
}
