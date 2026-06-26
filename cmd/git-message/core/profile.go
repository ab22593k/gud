package core

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gud/internal/profile"

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

var profileListCmd = &cobra.Command{
	Use:   "list",
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

	type category struct {
		name  string
		count int
	}

	catMap := make(map[string][]profile.CatalogEntry)
	for _, e := range entries {
		catMap[e.WorkMode] = append(catMap[e.WorkMode], e)
	}

	var cats []category
	for name, list := range catMap {
		cats = append(cats, category{name, len(list)})
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].name < cats[j].name })

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nFound %d profiles in %d categories:\n", len(entries), len(cats))
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	for _, cat := range cats {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %s (%d profiles)\n", cat.name, cat.count)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout())
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Use 'message --profile <slug>' or 'profile save <slug>' "+
		"with one of the slugs below.")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	// Show all entries with their slugs
	currentWorkMode := ""
	for _, e := range entries {
		if e.WorkMode != currentWorkMode {
			currentWorkMode = e.WorkMode
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n  [%s]\n", currentWorkMode)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "    %-50s %s\n", e.Slug, truncate(e.Summary, 70))
	}

	return nil
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

		content, err := profileManager.FetchProfile(context.Background(), slug)
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}

		if err := profileManager.Save(slug, profile.Profile{
			Profession: slug,
			Content:    content,
		}); err != nil {
			return fmt.Errorf("save: %w", err)
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
  gud profile show computer-scientist`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initProfileManager()
		slug := args[0]

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

	profileCmd.AddCommand(profileListCmd)
	profileCmd.AddCommand(profileSaveCmd)
	profileCmd.AddCommand(profileRemoveCmd)
	profileCmd.AddCommand(profileShowCmd)
}
