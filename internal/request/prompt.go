package request

// BuildCommitMessagePrompt creates a prompt for generating a git commit message.
// It includes the diff, optional commit context, detail level, focus hint, and persona style.
func BuildCommitMessagePrompt(diff, commitContext string, detailLevel DetailLevel, hint string, persona PersonaName) string {
	p := GetPersona(persona)
	return p.BuildPrompt(detailLevel, hint, commitContext, diff)
}
