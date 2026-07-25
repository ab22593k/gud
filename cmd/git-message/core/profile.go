package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gud/internal/profile"
	"gud/internal/tui"

	"github.com/spf13/cobra"
)

const profileCmdName = "profile"

var profileManager *profile.Manager

func initProfileManager() {
	if profileManager != nil {
		return
	}
	var err error
	profileManager, err = profile.NewManager()
	if err != nil {
		panic(fmt.Sprintf("init profile manager: %v", err))
	}
}

var profileCmd = &cobra.Command{
	Use:   profileCmdName,
	Short: "Manage AI agent profiles",
	Long: `Manage AI agent profiles from the K-Dense-AI/scientific-agents catalog (500+ profiles).

Use 'gud profile list --remote' to browse all available profiles.
Use 'gud profile save <slug>' to download and cache a profile.
Use 'gud message --profile <slug>' to generate a message with that profile.`,
}

const profileListCmdName = "list"

var profileListCmd = &cobra.Command{
	Use:   profileListCmdName,
	Short: "List available profiles",
	RunE: func(cmd *cobra.Command, _ []string) error {
		initProfileManager()

		showRemote, _ := cmd.Flags().GetBool("remote")

		if showRemote {
			return listRemoteProfiles(cmd)
		}

		profiles, err := profileManager.List()
		if err != nil {
			return fmt.Errorf("list profiles: %w", err)
		}

		if len(profiles) == 0 {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No cached profiles found. "+
				"Use 'gud profile list --remote' to browse available profiles, "+
				"then 'gud profile save <slug>' to install one.")

			return nil
		}

		// In terminal mode, launch the interactive TUI profile picker directly.
		if file, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(file) {
			return runLocalTUIPicker(cmd, profiles)
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Cached profiles:")
		_, _ = fmt.Fprintln(cmd.OutOrStdout())

		for _, p := range profiles {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %-30s %s\n", p.Slug, p.Profession)
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Use 'gud profile list --remote' to browse all available profiles.")

		return nil
	},
}

// runLocalTUIPicker launches the TUI profile list for locally cached profiles.
func runLocalTUIPicker(cmd *cobra.Command, profiles []profile.Profile) error {
	cached := make(map[string]bool, len(profiles))
	entries := make([]profile.CatalogEntry, len(profiles))

	for i, p := range profiles {
		cached[p.Slug] = true
		workMode := p.WorkMode
		if workMode == "" {
			workMode = "Cached"
		}
		entries[i] = profile.CatalogEntry{
			Slug:       p.Slug,
			Profession: p.Profession,
			WorkMode:   workMode,
			Summary:    p.Content,
		}
	}

	selected, err := tui.RunPicker(entries, nil, cached, "GUD Cached Profiles")
	if err != nil {
		return fmt.Errorf("picker: %w", err)
	}
	if selected != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"\nProfile %q selected. Use: gud message --profile %s\n",
			selected.Slug, selected.Slug)
	}

	return nil
}

func listRemoteProfiles(cmd *cobra.Command) error {
	initProfileManager()

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Fetching profile catalog from K-Dense-AI/scientific-agents...")

	entries, err := profileManager.FetchCatalog(context.Background())
	if err != nil {
		return fmt.Errorf("fetch catalog: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].WorkMode != entries[j].WorkMode {
			return entries[i].WorkMode < entries[j].WorkMode
		}

		return entries[i].Profession < entries[j].Profession
	})

	// In terminal mode, launch the interactive TUI picker directly.
	if file, ok := cmd.InOrStdin().(*os.File); ok && isTerminal(file) {
		return runTUIPicker(cmd, entries)
	}

	// Non-terminal: print the catalog listing to stdout.
	cats := categorizeByWorkMode(entries)
	printProfileSummary(cmd.OutOrStdout(), len(entries), cats)
	printDetailedEntries(cmd.OutOrStdout(), entries)

	return nil
}

// runTUIPicker launches the TUI profile picker and saves the selected profile.
func runTUIPicker(cmd *cobra.Command, entries []profile.CatalogEntry) error {
	// Build the set of already-cached slugs for cache indicators in the TUI.
	cached := make(map[string]bool)
	if cachedList, err := profileManager.List(); err == nil {
		for _, p := range cachedList {
			cached[p.Slug] = true
		}
	}

	download := func(ctx context.Context, slug string) error {
		profession := findProfession(slug, entries)

		return downloadAndSaveProfile(ctx, slug, profession)
	}

	selected, err := tui.RunPicker(entries, download, cached)
	if err != nil {
		return fmt.Errorf("picker: %w", err)
	}
	if selected != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(),
			"\nProfile %q saved. Use: gud message --profile %s\n",
			selected.Slug, selected.Slug)
	}

	return nil
}

// downloadAndSaveProfile fetches a profile from the remote catalog and
// caches it locally. Returns nil if already cached or successfully saved.
func downloadAndSaveProfile(ctx context.Context, slug, profession string) error {
	if profileManager.IsCached(slug) {
		return nil
	}

	content, err := profileManager.FetchProfile(ctx, slug)
	if err != nil {
		return fmt.Errorf("fetch profile %q: %w", slug, err)
	}

	if err := profileManager.Save(slug, profile.Profile{
		Profession: profession,
		Content:    content,
	}); err != nil {
		return fmt.Errorf("save profile %q: %w", slug, err)
	}

	return nil
}

// findProfession looks up the profession for a slug in the entries list.
// Returns empty string if not found.
func findProfession(slug string, entries []profile.CatalogEntry) string {
	for _, e := range entries {
		if e.Slug == slug {
			return e.Profession
		}
	}

	return ""
}

// category groups profiles by their work mode for display.
type category struct {
	name  string
	count int
}

// categorizeByWorkMode groups entries by WorkMode and returns a sorted list
// of categories (ordered by name), each with its profile count.
func categorizeByWorkMode(entries []profile.CatalogEntry) []category {
	catMap := make(map[string][]profile.CatalogEntry)
	for _, e := range entries {
		catMap[e.WorkMode] = append(catMap[e.WorkMode], e)
	}

	cats := make([]category, 0, len(catMap))
	for name, list := range catMap {
		cats = append(cats, category{name, len(list)})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].name < cats[j].name })

	return cats
}

// printProfileSummary prints the category summary header with profile counts
// and an instruction line for using profiles.
func printProfileSummary(w io.Writer, total int, cats []category) {
	_, _ = fmt.Fprintf(w, "\nFound %d profiles in %d categories:\n", total, len(cats))
	_, _ = fmt.Fprintln(w)

	for _, cat := range cats {
		_, _ = fmt.Fprintf(w, "  %s (%d profiles)\n", cat.name, cat.count)
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Use 'message --profile <slug>' or 'profile save <slug>' "+
		"with one of the slugs below.")
	_, _ = fmt.Fprintln(w)
}

// printDetailedEntries prints all profile entries grouped by work mode,
// with work mode headers separating the groups.
func printDetailedEntries(w io.Writer, entries []profile.CatalogEntry) {
	currentWorkMode := ""
	for _, e := range entries {
		if e.WorkMode != currentWorkMode {
			currentWorkMode = e.WorkMode
			_, _ = fmt.Fprintf(w, "\n  [%s]\n", currentWorkMode)
		}
		_, _ = fmt.Fprintf(w, "    %-50s %s\n", e.Slug, truncate(e.Summary, 70))
	}
}

var profileSaveCmd = &cobra.Command{
	Use:   "save <slug>",
	Short: "Download and cache a remote profile",
	Args:  cobra.ExactArgs(1),
	Long: `Download a scientific agent profile from K-Dense-AI/scientific-agents and cache it locally.

After saving, use it with: gud message --profile <slug>

Run 'gud profile list --remote' to see all available slugs.`,
	Example: `  gud profile save astrophysicist
  gud profile save computer-scientist
  gud profile save molecular-biologist`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initProfileManager()
		slug := args[0]

		if profileManager.IsCached(slug) {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Profile %q is already cached.\n", slug)

			return nil
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Downloading profile %q...\n", slug)
		if err := downloadAndSaveProfile(context.Background(), slug, slug); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Profile %q saved successfully.\n", slug)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Use: gud message --profile %s\n", slug)

		return nil
	},
}

var profileRemoveCmd = &cobra.Command{
	Use:     "remove <slug>",
	Short:   "Remove a cached profile",
	Args:    cobra.ExactArgs(1),
	Example: `  gud profile remove astrophysicist`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initProfileManager()
		slug := args[0]

		if err := profileManager.Remove(slug); err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Profile %q removed.\n", slug)

		return nil
	},
}

var profileShowCmd = &cobra.Command{
	Use:   "show <slug>",
	Short: "Show details of a profile",
	Args:  cobra.ExactArgs(1),
	Example: `  gud profile show astrophysicist
  gud profile show computer-scientist
  gud profile show astrophysicist --remote`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initProfileManager()
		slug := args[0]

		showRemote, _ := cmd.Flags().GetBool("remote")

		if showRemote {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Fetching profile %q from remote...\n", slug)
			content, err := profileManager.FetchProfile(context.Background(), slug)
			if err != nil {
				return fmt.Errorf("fetch remote: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Profile: %s (remote)\n", slug)
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), content)
			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Use 'gud profile save %s' to cache it locally.\n", slug)

			return nil
		}

		p, err := profileManager.Get(slug)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Profile: %s (cached)\n", p.Slug)
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
		if p.Content != "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), truncate(p.Content, 2000))
		}

		return nil
	},
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}

	return strings.TrimSpace(s[:maxLen]) + "..."
}

func init() {
	profileListCmd.Flags().BoolP("remote", "r", false, "List all remote profiles from K-Dense-AI/scientific-agents")
	profileShowCmd.Flags().BoolP("remote", "r", false, "Show a remote profile without saving it")

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileSaveCmd)
	profileCmd.AddCommand(profileRemoveCmd)
	profileCmd.AddCommand(profileShowCmd)
}
