package request

import "strings"

type Example struct {
	Diff   string
	Output string
}

type DetailLevelRules struct {
	Minimal  string
	Standard string
	Detailed string
}

type PersonaConfig struct {
	Name     string
	System   string
	Examples []Example
	Rules    DetailLevelRules
}

var personas = map[string]PersonaConfig{
	"embedded": {
		Name: "embedded",
		System: `# PERSONA
You are a Principal Embedded Kernel Maintainer. You are technically rigorous, demanding, and believe that a commit message is a permanent piece of technical documentation. You expect developers to explain *why* a change is necessary with absolute precision.

**Formatting Constraints (STRICT):**
- **Subject Line:** Maximum 72 characters.
- **Body Content:** Wrap all lines at exactly 82 characters. This is a hard limit
for mailing list compatibility and readability.`,
		Rules: DetailLevelRules{
			Minimal:  "EXIGENCY: Keep it technical and concise. A subsystem subject and a single paragraph of technical reasoning.",
			Standard: "EXIGENCY: Provide a multi-paragraph technical justification explaining the problem and solution.",
			Detailed: "EXIGENCY: Exhaustive technical documentation. Explain the state before/after, the logic flow, and architectural implications.",
		},
	},
}

func GetPersona(name string) PersonaConfig {
	if name == "" {
		name = "embedded"
	}
	if p, ok := personas[name]; ok {
		return p
	}
	return personas["embedded"]
}

func (p PersonaConfig) BuildPrompt(detailLevel, hint, additionalContext, diff string) string {
	var sb strings.Builder

	sb.WriteString(p.System)
	sb.WriteString("\n\n")

	if len(p.Examples) > 0 {
		sb.WriteString("Examples:\n")
		for i, ex := range p.Examples {
			sb.WriteString(ex.Diff)
			sb.WriteString("\n")
			sb.WriteString(ex.Output)
			if i < len(p.Examples)-1 {
				sb.WriteString("\n\n")
			}
		}
		sb.WriteString("\n\n")
	}

	switch detailLevel {
	case "minimal":
		sb.WriteString(p.Rules.Minimal)
	case "detailed":
		sb.WriteString(p.Rules.Detailed)
	default:
		sb.WriteString(p.Rules.Standard)
	}
	sb.WriteString("\n")

	if hint != "" {
		sb.WriteString("Focus: ")
		sb.WriteString(hint)
		sb.WriteString("\n")
	}

	if additionalContext != "" {
		sb.WriteString("Context: ")
		sb.WriteString(additionalContext)
		sb.WriteString("\n")
	}

	sb.WriteString("\nDiff:\n")
	sb.WriteString(diff)
	sb.WriteString("\n\nOutput:\n")

	return sb.String()
}
