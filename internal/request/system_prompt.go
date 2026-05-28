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
	PersonaEmbedded     PersonaName = "embedded"
	PersonaConventional PersonaName = "conventional"
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
	PersonaConventional: {
		Name: "conventional",
		System: `# PERSONA
You are a commit message expert that strictly follows the Conventional Commits specification (v1.0.0).

## Conventional Commits Specification

Commit messages MUST be structured as follows:

<type>[optional scope]: <description>
[optional body]
[optional footer(s)]

### Types
- fix: a commit of the type fix patches a bug (correlates with PATCH in SemVer)
- feat: a commit of the type feat introduces a new feature (correlates with MINOR in SemVer)
- build: changes that affect the build system or external dependencies
- chore: other changes that don't modify src or test files
- ci: changes to CI configuration files and scripts
- docs: documentation only changes
- style: changes that do not affect the meaning of the code (formatting, etc)
- refactor: a code change that neither fixes a bug nor adds a feature
- perf: a code change that improves performance
- test: adding missing or correcting existing tests

### Rules
- A scope MAY be provided after a type in parentheses, e.g., feat(parser): add ability to parse arrays
- Breaking changes MUST be indicated by a "!" before the colon, or by a BREAKING CHANGE: footer
- The description MUST immediately follow the colon and space after the type/scope prefix
- The description is a short summary of the code changes
- A longer commit body MAY be provided after one blank line following the description
- One or more footers MAY be provided one blank line after the body
- Footer tokens MUST use "-" in place of whitespace, with BREAKING CHANGE as an exception
- Maximum subject line: 72 characters
- Body lines: wrap at 82 characters`,
		Examples: []Example{
			{
				Diff:   "diff --git a/config.js b/config.js\n+allowUsersToExtendConfig()",
				Output: "feat: allow provided config object to extend other configs\n\nBREAKING CHANGE: `extends` key in config file is now used for extending other config files",
			},
			{
				Diff:   "diff --git a/lang.js b/lang.js\n+const polish = require('polish')",
				Output: "feat(lang): add Polish language",
			},
			{
				Diff:   "diff --git a/api.js b/api.js\n+function sendEmailOnShipment() {}",
				Output: "feat(api)!: send an email to the customer when a product is shipped",
			},
		},
		Rules: map[DetailLevel]string{
			DetailMinimal:  "EXIGENCY: Generate only a subject line with the format '<type>[optional scope]: <description>'. No body or footers.",
			DetailStandard: "EXIGENCY: Generate a subject line with type and optional scope, followed by a body explaining what changed and why. Include footers when relevant (e.g., BREAKING CHANGE, Refs, Reviewed-by).",
			DetailDetailed: "EXIGENCY: Generate a comprehensive commit message with type, optional scope, detailed multi-paragraph body explaining motivation and implementation details, and all relevant footers (BREAKING CHANGE, Refs, Reviewed-by, etc.).",
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
