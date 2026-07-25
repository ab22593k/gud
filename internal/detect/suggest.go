package detect

import (
	"fmt"
	"sort"
	"strings"

	"gud/internal/profile"
)

// SuggestProfile ranks catalog entries by how well they match the repo's
// file extension statistics. Returns top 3 entries sorted by score, or nil
// if no matches are found.
//
// The top 3 file extensions (by count) are used as keywords to match against
// catalog entry fields (profession, summary, work_mode) using case-insensitive
// substring comparison.
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

// topExtensionKeywords returns the top 3 extensions with leading dot stripped.
func topExtensionKeywords(stats *RepoStats) []string {
	type extItem struct {
		ext   string
		count int
	}
	var exts []extItem
	for ext, count := range stats.FilesByExtension {
		exts = append(exts, extItem{ext, count})
	}
	sort.Slice(exts, func(i, j int) bool {
		if exts[i].count != exts[j].count {
			return exts[i].count > exts[j].count
		}

		return exts[i].ext < exts[j].ext
	})

	var keywords []string
	for i := 0; i < 3 && i < len(exts); i++ {
		kw := strings.TrimPrefix(exts[i].ext, ".")
		if kw != "" {
			keywords = append(keywords, strings.ToLower(kw))
		}
	}

	return keywords
}

// scoreEntry counts how many keywords appear in the entry's combined text.
func scoreEntry(entry profile.CatalogEntry, keywords []string) int {
	entryText := strings.ToLower(entry.Profession + " " + entry.Summary + " " + entry.WorkMode)
	score := 0
	for _, kw := range keywords {
		if strings.Contains(entryText, kw) {
			score++
		}
	}

	return score
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
