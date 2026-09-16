package detect

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"gud/internal/profile"
)

// SuggestProfile ranks catalog entries by how well they match the repo's
// file extension statistics. Returns top 3 entries sorted by score, or nil
// if no matches are found.
//
// The top 3 file extensions (by count, excluding extensionless files) are
// used as keywords to match against catalog entry fields (profession,
// summary, work_mode) using case-insensitive word or word-prefix comparison.
func SuggestProfile(stats *RepoStats, catalog []profile.CatalogEntry) []profile.CatalogEntry {
	if stats == nil || stats.TotalFiles == 0 || len(catalog) == 0 {
		return nil
	}

	keywords := topExtensionKeywords(stats)
	if len(keywords) == 0 {
		return nil
	}

	type scored struct {
		entry profile.CatalogEntry
		score int
	}

	var scoredEntries []scored

	for _, entry := range catalog {
		score := scoreEntry(entry, keywords)
		if score > 0 {
			scoredEntries = append(scoredEntries, scored{entry, score})
		}
	}

	if len(scoredEntries) == 0 {
		return nil
	}

	sort.Slice(scoredEntries, func(i, j int) bool {
		if scoredEntries[i].score != scoredEntries[j].score {
			return scoredEntries[i].score > scoredEntries[j].score
		}

		return scoredEntries[i].entry.Slug < scoredEntries[j].entry.Slug
	})

	n := min(3, len(scoredEntries))

	result := make([]profile.CatalogEntry, n)
	for i := range n {
		result[i] = scoredEntries[i].entry
	}

	return result
}

// topExtensionKeywords returns up to 3 extensions with leading dot stripped,
// lowercased for matching. Files without an extension (noExtensionKey) are
// skipped: the literal never matches catalog text and would waste a slot.
func topExtensionKeywords(stats *RepoStats) []string {
	type extItem struct {
		ext   string
		count int
	}

	exts := make([]extItem, 0, len(stats.FilesByExtension))
	for ext, count := range stats.FilesByExtension {
		exts = append(exts, extItem{ext, count})
	}

	sort.Slice(exts, func(i, j int) bool {
		if exts[i].count != exts[j].count {
			return exts[i].count > exts[j].count
		}

		return exts[i].ext < exts[j].ext
	})

	keywords := make([]string, 0, 3)

	for _, e := range exts {
		if len(keywords) >= 3 {
			break
		}

		if e.ext == noExtensionKey {
			continue
		}

		kw := strings.ToLower(strings.TrimPrefix(e.ext, "."))
		if kw == "" {
			continue
		}

		keywords = append(keywords, kw)
	}

	return keywords
}

// scoreEntry counts how many keywords match the entry's combined text.
// Matching is whole-word or word-prefix (e.g. "py" matches "python") to catch
// common extension abbreviations without false positives like "go"
// substring-matching "django". Single-letter keywords require an exact word to
// avoid matching every word with that initial.
func scoreEntry(entry profile.CatalogEntry, keywords []string) int {
	words := wordSet(strings.ToLower(entry.Profession + " " + entry.Summary + " " + entry.WorkMode))
	score := 0

	for _, kw := range keywords {
		if matchesKeyword(words, kw) {
			score++
		}
	}

	return score
}

// matchesKeyword reports whether kw matches any word exactly or, for
// multi-letter keywords, as a word prefix.
func matchesKeyword(words map[string]struct{}, kw string) bool {
	if kw == "" {
		return false
	}

	if _, ok := words[kw]; ok {
		return true
	}

	if len(kw) < 2 {
		return false
	}

	for w := range words {
		if strings.HasPrefix(w, kw) {
			return true
		}
	}

	return false
}

// wordSet splits text into a set of maximal letter/digit runs.
func wordSet(text string) map[string]struct{} {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	set := make(map[string]struct{}, len(fields))
	for _, w := range fields {
		if w != "" {
			set[w] = struct{}{}
		}
	}

	return set
}

// FormatSuggestionMessage formats the interactive prompt for profile selection.
// Returns empty string if suggestions is empty.
func FormatSuggestionMessage(suggestions []profile.CatalogEntry) string {
	if len(suggestions) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\nNo AI profile configured for this repository.\n\n")
	sb.WriteString("Based on your project's file types, these profiles may be relevant:\n\n")

	for i, s := range suggestions {
		summary := s.Summary

		const maxSummary = 60
		if len(summary) > maxSummary {
			summary = summary[:maxSummary-3] + "..."
		}

		fmt.Fprintf(&sb, "  [%d] %-35s %s\n", i+1, s.Slug, summary)
	}

	sb.WriteString("\nSelect a profile (1-3), or [s]kip this suggestion, or [a]bort: ")

	return sb.String()
}
