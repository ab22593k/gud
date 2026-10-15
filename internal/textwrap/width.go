// Package textwrap measures and folds plain text to a terminal-cell width.
//
// It is the single width implementation shared by the full-screen review
// (internal/tui) and the headless presentation paths (cmd/git-message/core),
// so FR-013 holds on every path: one measure, one wrapper, no copies.
package textwrap

import (
	"strings"

	"github.com/clipperhouse/displaywidth"
)

// Columns reports the width of s in terminal cells: CJK characters occupy
// two cells and emoji grapheme clusters vary, so byte counting would
// misjudge them.
func Columns(s string) int {
	return displaywidth.String(s)
}

// Wrap folds lines in text to max display width limit, measured in terminal
// cells via Columns. Lines already within limit cells are left unchanged.
// Leading indentation (spaces or tabs) is preserved on continuation lines.
// Words stay atomic: a single word longer than limit is never split
// mid-token, so Wrap folds but never truncates content. A non-positive limit
// returns text unchanged.
func Wrap(text string, limit int) string {
	if limit <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")

	var result []string

	for _, line := range lines {
		if Columns(line) <= limit {
			result = append(result, line)

			continue
		}

		// Preserve leading indentation (spaces or tabs)
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		indentWidth := Columns(indent)

		words := strings.Fields(line)
		if len(words) == 0 {
			result = append(result, line)

			continue
		}

		var currentLine strings.Builder

		currentLine.WriteString(indent)
		currentLine.WriteString(words[0])

		currentWidth := indentWidth + Columns(words[0])

		for _, word := range words[1:] {
			wordWidth := Columns(word)
			if currentWidth+1+wordWidth > limit {
				result = append(result, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(indent)
				currentLine.WriteString(word)

				currentWidth = indentWidth + wordWidth
			} else {
				currentLine.WriteString(" ")
				currentLine.WriteString(word)

				currentWidth += 1 + wordWidth
			}
		}

		if currentLine.Len() > 0 {
			result = append(result, currentLine.String())
		}
	}

	return strings.Join(result, "\n")
}
