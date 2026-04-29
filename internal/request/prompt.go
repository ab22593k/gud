package request

import "strings"

// BuildCommitMessagePrompt creates a prompt for generating a git commit message.
// It includes the diff, optional context, detail level, and focus hint.
func BuildCommitMessagePrompt(diff, context, detailLevel, hint string) string {
	var prompt strings.Builder

	prompt.WriteString("Generate a git commit message for the following changes.\n\n")

	// Add detail level instructions
	switch detailLevel {
	case "minimal":
		prompt.WriteString("Rules:\n")
		prompt.WriteString("- Use conventional commit format (feat:, fix:, chore:, docs:, etc.)\n")
		prompt.WriteString("- Keep the message under 50 characters (very short)\n")
		prompt.WriteString("- Just the subject line, no body\n")
	case "detailed":
		prompt.WriteString("Rules:\n")
		prompt.WriteString("- Use conventional commit format (feat:, fix:, chore:, docs:, etc.)\n")
		prompt.WriteString("- Include a subject line under 72 characters\n")
		prompt.WriteString("- Include a body explaining what changed and why\n")
		prompt.WriteString("- Be specific and comprehensive\n")
	default: // standard
		prompt.WriteString("Rules:\n")
		prompt.WriteString("- Use conventional commit format (feat:, fix:, chore:, docs:, etc.)\n")
		prompt.WriteString("- Keep the message under 72 characters for the subject line\n")
		prompt.WriteString("- Be specific about what changed and why\n")
		prompt.WriteString("- Do not include any explanation, just the commit message\n")
	}

	// Add hint if provided
	if hint != "" {
		prompt.WriteString("\nFocus on: ")
		prompt.WriteString(hint)
		prompt.WriteString("\n")
	}

	// Add context if provided
	if context != "" {
		prompt.WriteString("\nAdditional context: ")
		prompt.WriteString(context)
		prompt.WriteString("\n")
	}

	// Add diff
	prompt.WriteString("\nGit diff:\n```\n")
	prompt.WriteString(diff)
	prompt.WriteString("\n```\n\n")
	prompt.WriteString("Commit message:")

	return prompt.String()
}
