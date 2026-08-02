// Package git provides a thin wrapper around the git CLI for reading staged
// diffs, extracting code units, and installing git hooks.
package git

import (
	"regexp"
	"strings"
)

// CodeUnit represents a named code element (function, method, struct, type)
// extracted from the hunk headers of a git diff.
type CodeUnit struct {
	Name       string
	Kind       string // "function", "method", "struct", "type"
	ChangeType string // "modified", "added", "removed"
	FilePath   string
}

// changeTypeModified is the change type assigned to code units parsed from hunk
// headers, which do not carry added/removed information.
const changeTypeModified = "modified"

// hunkHeaderWithDecl matches hunk headers that include an inline declaration:
//
//	@@ -10,7 +10,8 @@ func NewClient(...)
//	@@ -30,7 +30,7 @@ func (g *GitRepo) Commit(...)
//	@@ -1,5 +1,6 @@ type Config struct
var hunkHeaderWithDecl = regexp.MustCompile(
	`@@\s+-(\d+),?\d*\s+\+(\d+),?\d*\s+@@\s+(func\s+(?:\([^)]*\)\s+)?\w+|type\s+\w+)`,
)

// goDeclRegex matches Go top-level declarations appearing as context lines in
// a hunk. These are lines that start with space (context), -, or + and contain
// func/type declarations.
var goDeclRegex = regexp.MustCompile(`^[\s\-+]?(func\s+(?:\([^)]*\)\s+)?\w+|type\s+\w+)`)

// ExtractCodeUnits parses a git diff and returns the list of code units whose
// definitions appear in hunk headers or context lines. Only Go diffs are
// currently supported. Returns nil if no code units can be identified.
func ExtractCodeUnits(diff string) []CodeUnit {
	var units []CodeUnit

	for _, entry := range splitDiffEntries(diff) {
		filePath := extractFilePath(entry)
		hunks := splitHunks(entry)

		for _, hunk := range hunks {
			decl := extractDeclaration(hunk)
			if decl == "" {
				continue
			}
			unit := parseDeclaration(decl, filePath)
			if unit != nil {
				units = append(units, *unit)
			}
		}
	}

	return units
}

// splitHunks splits a single-file diff entry into its constituent hunks.
func splitHunks(entry string) []string {
	lines := strings.Split(entry, "\n")
	var hunks []string
	var current []string
	inHunk := false

	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if inHunk && len(current) > 0 {
				hunks = append(hunks, strings.Join(current, "\n"))
			}
			current = []string{line}
			inHunk = true
		} else if inHunk {
			current = append(current, line)
		}
	}
	if inHunk && len(current) > 0 {
		hunks = append(hunks, strings.Join(current, "\n"))
	}

	return hunks
}

// extractDeclaration looks for a Go declaration in a hunk. It first checks
// the hunk header line for an inline declaration; failing that, it scans
// the hunk body (context/added/removed lines) for the first declaration.
func extractDeclaration(hunk string) string {
	lines := strings.Split(hunk, "\n")
	if len(lines) == 0 {
		return ""
	}

	// Check the hunk header line (first line, starts with @@).
	m := hunkHeaderWithDecl.FindStringSubmatch(lines[0])
	if len(m) > 3 && m[3] != "" {
		return m[3]
	}

	// Scan body lines for a Go declaration.
	for _, line := range lines[1:] {
		if line == "" {
			continue
		}
		trimmed := strings.TrimSpace(line)
		// The hunk body starts with the hunk content. We need to look at
		// context lines (starting with space), added lines (+), or removed lines (-).
		m = goDeclRegex.FindStringSubmatch(trimmed)
		if len(m) > 1 && m[1] != "" {
			return m[1]
		}
		// Also try matching the raw (possibly prefixed) line directly.
		if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") {
			return trimmed
		}
	}

	return ""
}

// parseDeclaration extracts name and kind from a hunk header declaration.
func parseDeclaration(decl, filePath string) *CodeUnit {
	trimmed := strings.TrimSpace(decl)

	switch {
	case strings.HasPrefix(trimmed, "func "):
		sig := strings.TrimPrefix(trimmed, "func ")

		return parseFuncOrMethod(sig, filePath)

	case strings.HasPrefix(trimmed, "type "):
		name := strings.TrimPrefix(trimmed, "type ")
		name = strings.Fields(name)[0]
		kind := "type"
		if len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
			kind = "struct"
		}
		return &CodeUnit{Name: name, Kind: kind, ChangeType: changeTypeModified, FilePath: filePath}
	}

	return nil
}

// parseFuncOrMethod handles "func Name(...)" or "func (recv) Name(...)".
func parseFuncOrMethod(sig, filePath string) *CodeUnit {
	sig = strings.TrimSpace(sig)

	if strings.HasPrefix(sig, "(") {
		closeParen := strings.Index(sig, ")")
		if closeParen == -1 || closeParen+1 >= len(sig) {
			return nil
		}
		afterParen := strings.TrimSpace(sig[closeParen+1:])
		funcName := extractIdentifier(afterParen)
		if funcName == "" {
			return nil
		}

		return &CodeUnit{
			Name: "(*" + extractReceiverType(sig[1:closeParen]) + ")." + funcName,
			Kind: "method", ChangeType: changeTypeModified, FilePath: filePath,
		}
	}

	funcName := extractIdentifier(sig)
	if funcName == "" {
		return nil
	}
	return &CodeUnit{Name: funcName, Kind: "function", ChangeType: "modified", FilePath: filePath}
}

// extractIdentifier returns the first word/identifier in s.
func extractIdentifier(s string) string {
	s = strings.TrimSpace(s)
	idx := strings.IndexAny(s, " (")
	if idx == -1 {
		return s
	}
	return s[:idx]
}

// extractReceiverType extracts the type name from a receiver string.
func extractReceiverType(recv string) string {
	parts := strings.Fields(strings.TrimSpace(recv))
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimPrefix(parts[len(parts)-1], "*")
}

// splitDiffEntries splits a multi-file diff into per-file entries.
func splitDiffEntries(diff string) []string {
	entries := strings.Split(diff, "\ndiff --git ")
	var result []string
	for i, e := range entries {
		if i == 0 {
			if strings.TrimSpace(e) != "" {
				result = append(result, e)
			}
		} else {
			result = append(result, "diff --git "+e)
		}
	}
	return result
}

// extractFilePath extracts the "+++ b/..." path from a diff entry.
func extractFilePath(entry string) string {
	for _, line := range strings.Split(entry, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			return strings.TrimPrefix(line, "+++ b/")
		}
	}
	for _, line := range strings.Split(entry, "\n") {
		if strings.HasPrefix(line, "--- a/") {
			return strings.TrimPrefix(line, "--- a/")
		}
	}
	return ""
}
