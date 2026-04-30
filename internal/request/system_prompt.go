package request

import "strings"

type Example struct {
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
	PersonaEmbedded PersonaName = "embedded"
)

type PersonaConfig struct {
	Name     string
	System   string
	Examples []Example
	Rules    map[DetailLevel]string
}

var personas = map[PersonaName]PersonaConfig{
	PersonaEmbedded: {
		Name: "embedded",
		System: `# PERSONA
You are a Principal Embedded Kernel Maintainer. You are technically rigorous, demanding, and believe that a commit message is a permanent piece of technical documentation. You expect developers to explain *why* a change is necessary with absolute precision.

**Formatting Constraints (STRICT):**
- **Subject Line:** Maximum 72 characters.
- **Body Content:** Wrap all lines at exactly 82 characters. This is a hard limit
for mailing list compatibility and readability.`,
		Rules: map[DetailLevel]string{
			DetailMinimal:  "EXIGENCY: Keep it technical and concise. A subsystem subject and a single paragraph of technical reasoning.",
			DetailStandard: "EXIGENCY: Provide a multi-paragraph technical justification explaining the problem and solution.",
			DetailDetailed: "EXIGENCY: Exhaustive technical documentation. Explain the state before/after, the logic flow, and architectural implications.",
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
	writeExamples(&sb, p.Examples)
	writeDetailRules(&sb, detailLevel, p.Rules)
	writeHint(&sb, hint)
	writeContext(&sb, context)
	writeDiff(&sb, diff)
	return sb.String()
}

func writeSystem(sb *strings.Builder, p PersonaConfig) {
	sb.WriteString(p.System)
	sb.WriteString("\n\n")
}

func writeExamples(sb *strings.Builder, examples []Example) {
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
	sb.WriteString("\n\n")
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
	sb.WriteString("\nDiff:\n")
	sb.WriteString(diff)
	sb.WriteString("\n\nOutput:\n")
}
