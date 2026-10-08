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
		validate    func(t *testing.T, prompt string)
	}{
		{
			name:        "basic diff creates prompt with diff",
			diff:        "diff --git a/main.go b/main.go\n+fmt.Println(\"hello\")",
			context:     "",
			detailLevel: DetailStandard,
			hint:        "",
			validate: func(t *testing.T, prompt string) {
				t.Helper()

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
			validate: func(t *testing.T, prompt string) {
				t.Helper()

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
			validate: func(t *testing.T, prompt string) {
				t.Helper()

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
			validate: func(t *testing.T, prompt string) {
				t.Helper()

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
			validate: func(t *testing.T, prompt string) {
				t.Helper()

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
			validate: func(t *testing.T, prompt string) {
				t.Helper()

				if prompt == "" {
					t.Errorf("prompt should not be empty")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, task := BuildAgentPrompt(
				tt.diff, tt.context, tt.detailLevel, tt.hint, "", defaultWrapLine)
			tt.validate(t, task)
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
				t.Helper()

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
				t.Helper()

				if !strings.Contains(prompt, "72") && !strings.Contains(prompt, "82") {
					t.Errorf("detailed prompt should mention character limits")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, task := BuildAgentPrompt(
				tt.diff, tt.context, tt.detailLevel, tt.hint, "", defaultWrapLine)
			tt.validate(t, task)
		})
	}
}

func TestBuildCommitMessagePrompt_ContainsDeterministicRules(t *testing.T) {
	t.Parallel()

	_, prompt := BuildAgentPrompt(
		"diff --git a/main.go b/main.go", "", DetailStandard, "", "", defaultWrapLine)

	for _, want := range []string{"imperative", "never end the subject", "blank line", "appear in the diff"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt missing deterministic rule %q", want)
		}
	}
}

// Custom content is mounted as AGENTS.md, not inlined in the task: the two must
// never both carry it.
func TestBuildAgentPrompt_MountsCustomContentInsteadOfInlining(t *testing.T) {
	t.Parallel()

	const custom = "CUSTOM SYSTEM PROMPT FOR TESTING"

	instructions, task := BuildAgentPrompt(
		"diff --git a/main.go b/main.go", "", DetailStandard, "", custom, defaultWrapLine)

	if instructions != custom {
		t.Errorf("instructions = %q, want the custom content verbatim", instructions)
	}

	if strings.Contains(task, custom) {
		t.Errorf("custom content also inlined in the task, got:\n%s", task)
	}

	if strings.Contains(task, "permanent technical documentation") {
		t.Errorf("default system appended alongside custom content, got:\n%s", task)
	}
}

func TestBuildPromptWithContent_EmptyFallsBackToDefault(t *testing.T) {
	t.Parallel()

	_, prompt := BuildAgentPrompt(
		"diff --git a/main.go b/main.go", "", DetailStandard, "", "", defaultWrapLine)

	if !strings.Contains(prompt, "permanent technical documentation") {
		t.Errorf("empty content should fall back to default system, got:\n%s", prompt)
	}

	if strings.HasPrefix(prompt, "\n") {
		t.Errorf("fallback prompt should not start with blank line, got %q", prompt)
	}
}

func TestBuildCommitMessagePrompt_NoDuplicateDefault(t *testing.T) {
	t.Parallel()

	_, prompt := BuildAgentPrompt(
		"diff --git a/main.go b/main.go", "", DetailStandard, "", "", defaultWrapLine)

	if n := strings.Count(prompt, "permanent technical documentation"); n != 1 {
		t.Errorf("default system appears %d times, want exactly 1", n)
	}
}
