package request

// BuildCommitMessagePrompt creates a prompt for generating a git commit message.
// It includes the diff, optional additional context, detail level, focus hint, and persona style.
func BuildCommitMessagePrompt(diff, additionalContext, detailLevel, hint, persona string) string {
	p := GetPersona(persona)
	return p.BuildPrompt(detailLevel, hint, additionalContext, diff)
}
