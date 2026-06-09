package request

import "strings"

type Showcase struct {
	Diff   string
	Output string
}

// DetailLevel represents the level of detail for the commit message.
type DetailLevel string

const (
	DetailMinimal  DetailLevel = "minimal"
	DetailStandard DetailLevel = "standard"
	DetailDetailed DetailLevel = "detailed"
)

// PersonaName represents the name of a persona configuration.
type PersonaName string

const (
	PersonaEmbedded     PersonaName = "embedded"
	PersonaConventional PersonaName = "conventional"
)

type PersonaConfig struct {
	Name      string
	System    string
	Showcases []Showcase
	Rules     map[DetailLevel]string
}

var personas = map[PersonaName]PersonaConfig{
	PersonaEmbedded: {
		Name: "embedded",
		System: `You are an Embedded engineer. A commit message is permanent technical documentation. Explain *why* a change is necessary with precision.
Subject: max 72 chars. Body: wrap at 82 chars.

Respond in plain text only. Do NOT use markdown, code fences, backticks, or any formatting. Output ONLY the commit message itself — no preamble, no explanation, no commentary before or after.`,
		Rules: map[DetailLevel]string{
			DetailMinimal:  "Subject line + single paragraph of technical reasoning.",
			DetailDetailed: "Exhaustive docs: before/after state, logic flow, architectural implications.",
			DetailStandard: "Multi-paragraph technical justification: problem and solution.",
		},
	},
	PersonaConventional: {
		Name: "conventional",
		System: `Follow Conventional Commits v1.0.0.
Format: <type>[optional scope]: <description>
Types: fix, feat, build, chore, ci, docs, style, refactor, perf, test
Scope: optional, in parentheses after type, e.g. feat(parser): ...
Breaking: "!" before colon or BREAKING CHANGE: footer
Subject: max 72 chars. Body: wrap at 82 chars.
Body after blank line. Footers after blank line. Footer tokens use "-" not whitespace.

Respond in plain text only. Do NOT use markdown, code fences, backticks, or any formatting. Output ONLY the commit message itself — no preamble, no explanation, no commentary before or after.`,
		Showcases: []Showcase{
			{
				Diff:   "+allowUsersToExtendConfig()",
				Output: "feat: allow provided config object to extend other configs\n\nBREAKING CHANGE: `extends` key in config file is now used for extending other config files",
			},
			{
				Diff:   "+const polish = require('polish')",
				Output: "feat(lang): add Polish language",
			},
			{
				Diff:   "+function sendEmailOnShipment() {}",
				Output: "feat(api)!: send an email to the customer when a product is shipped",
			},
			{
				Diff:   "-deleteUser(id) +removeUser(id)",
				Output: "fix: rename deleteUser to removeUser for API consistency",
			},
		},
		Rules: map[DetailLevel]string{
			DetailMinimal:  "Subject only: '<type>[optional scope]: <description>'. No body or footers.",
			DetailStandard: "Subject + body explaining what changed and why. Include footers when relevant (BREAKING CHANGE, Refs, Reviewed-by).",
			DetailDetailed: "Subject + detailed multi-paragraph body with motivation and implementation, plus all relevant footers.",
		},
	},
}

// GetPersona returns the persona configuration for the given name.
// If the name is empty or not found, it returns the default "embedded" persona.
func GetPersona(name PersonaName) PersonaConfig {
	if name == "" {
		name = PersonaEmbedded
	}
	if p, ok := personas[name]; ok {
		return p
	}
	return personas[PersonaEmbedded]
}

// BuildPrompt creates a full prompt for generating a commit message.
func (p PersonaConfig) BuildPrompt(detailLevel DetailLevel, hint, context, diff string) string {
	var sb strings.Builder

	sb.WriteString(p.System)
	sb.WriteString("\n")
	writeExamples(&sb, p.Showcases)
	writeLabeled(&sb, "", ruleForLevel(detailLevel, p.Rules))
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

// writeExamples writes persona examples as a diff → output showcase.
func writeExamples(sb *strings.Builder, examples []Showcase) {
	if len(examples) == 0 {
		return
	}
	sb.WriteString("Examples:\n")
	for i, ex := range examples {
		sb.WriteString(ex.Diff)
		sb.WriteString("\n")
		sb.WriteString(ex.Output)
		if i < len(examples)-1 {
			sb.WriteString("\n\n")
		}
	}
	sb.WriteString("\n")
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
