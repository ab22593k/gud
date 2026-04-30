package request

// BuildCommitMessagePrompt creates a prompt for generating a git commit message.
// It includes the diff, optional context, detail level, focus hint, and persona style.
func BuildCommitMessagePrompt(diff, context, detailLevel, hint, persona string) string {
	p := GetPersona(persona)
	return p.BuildPrompt(detailLevel, hint, context, diff)
}
