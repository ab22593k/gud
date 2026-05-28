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
		System: `You are a Embedded engineer. A commit message is permanent technical documentation. Explain *why* a change is necessary with precision.
Subject: max 72 chars. Body: wrap at 82 chars.`,
		Rules: map[DetailLevel]string{
			DetailMinimal:  "Subject line + single paragraph of technical reasoning.",
			DetailStandard: "Multi-paragraph technical justification: problem and solution.",
			DetailDetailed: "Exhaustive docs: before/after state, logic flow, architectural implications.",
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
Body after blank line. Footers after blank line. Footer tokens use "-" not whitespace.`,
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
	writeSystem(&sb, p)
	writeExamples(&sb, p.Showcases)
	writeDetailRules(&sb, detailLevel, p.Rules)
	writeHint(&sb, hint)
	writeContext(&sb, context)
	writeDiff(&sb, diff)
	return sb.String()
}

func writeSystem(sb *strings.Builder, p PersonaConfig) {
	sb.WriteString(p.System)
	sb.WriteString("\n")
}

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

func writeDetailRules(sb *strings.Builder, level DetailLevel, rules map[DetailLevel]string) {
	rule, ok := rules[level]
	if !ok {
		rule = rules[DetailStandard]
	}
	sb.WriteString(rule)
	sb.WriteString("\n")
}

func writeHint(sb *strings.Builder, hint string) {
	if hint == "" {
		return
	}
	sb.WriteString("Focus: ")
	sb.WriteString(hint)
	sb.WriteString("\n")
}

func writeContext(sb *strings.Builder, context string) {
	if context == "" {
		return
	}
	sb.WriteString("Context: ")
	sb.WriteString(context)
	sb.WriteString("\n")
}

func writeDiff(sb *strings.Builder, diff string) {
	sb.WriteString("Diff:\n")
	sb.WriteString(diff)
	sb.WriteString("\nOutput:\n")
}
