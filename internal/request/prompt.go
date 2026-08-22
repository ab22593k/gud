package request

import (
	"fmt"
	"strings"
)

// DetailLevel represents the level of detail for the commit message.
type DetailLevel string

const (
	DetailMinimal  DetailLevel = "minimal"
	DetailStandard DetailLevel = "standard"
	DetailDetailed DetailLevel = "detailed"
)

// ProfileName represents the name of a profile configuration.
type ProfileName string

type ProfileConfig struct {
	Name   string
	System string
	Rules  map[DetailLevel]string
}

var defaultProfile = ProfileConfig{
	Name: "__default__",
	System: `A commit message is permanent technical documentation. Explain *why* a change is necessary with precision.

	Respond in plain text only. Do NOT use markdown, code fences, backticks, or any formatting. ` +
		`Output ONLY the commit message itself — no preamble, no explanation, no commentary before or after.`,
	Rules: map[DetailLevel]string{
		DetailMinimal:  "Subject line + single paragraph of technical reasoning",
		DetailDetailed: "Exhaustive docs: before/after state, logic flow, architectural implications",
		DetailStandard: "Multi-paragraph technical justification: problem and solution",
	},
}

const defaultWrapLine = 72

// BuildPrompt creates a full prompt for generating a commit message.
func (p ProfileConfig) BuildPrompt(detailLevel DetailLevel, hint, context, diff string) string {
	return p.BuildPromptWithContent(detailLevel, hint, context, diff, p.System, defaultWrapLine)
}

// BuildPromptWithContent creates a full prompt with a custom system prompt.
func (p ProfileConfig) BuildPromptWithContent(
	detailLevel DetailLevel, hint, context, diff, systemContent string, wrapLine int,
) string {
	var sb strings.Builder

	sb.WriteString(systemContent)
	sb.WriteString("\n")
	sb.WriteString(defaultProfile.System)
	sb.WriteString("\n")
	writeLabeled(&sb, "", ruleForLevel(detailLevel, p.Rules))
	fmt.Fprintf(&sb, "Wrap all lines at %d characters.\n", wrapLine)
	writeLabeled(&sb, "Focus: ", hint)
	writeLabeled(&sb, "Context: ", context)
	sb.WriteString("Diff:\n")
	sb.WriteString(diff)
	sb.WriteString("\nOutput:\n")

	return sb.String()
}

// ruleForLevel returns the rule string for the given detail level, falling back
// to DetailStandard if the level is not found.
func ruleForLevel(level DetailLevel, rules map[DetailLevel]string) string {
	rule, ok := rules[level]
	if !ok {
		rule = rules[DetailStandard]
	}

	return rule
}

// writeLabeled writes content prefixed with label, but only if content is
// non-empty. If label is empty, it writes content directly.
func writeLabeled(sb *strings.Builder, label, content string) {
	if content == "" {
		return
	}
	sb.WriteString(label)
	sb.WriteString(content)
	sb.WriteString("\n")
}

// BuildCommitMessagePrompt creates a prompt for generating a git commit message.
func BuildCommitMessagePrompt(
	diff, commitContext string, detailLevel DetailLevel, hint string, profile ProfileName,
) string {
	p := defaultProfile

	return p.BuildPrompt(detailLevel, hint, commitContext, diff)
}

// BuildCommitMessagePromptWithContent creates a prompt using the provided system
// content. If content is empty, falls back to the default profile.
func BuildCommitMessagePromptWithContent(
	diff, commitContext string, detailLevel DetailLevel, hint string, _ ProfileName, systemContent string, wrapLine int,
) string {
	p := defaultProfile

	return p.BuildPromptWithContent(detailLevel, hint, commitContext, diff, systemContent, wrapLine)
}
