package core

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"gud/internal/wire"

	"github.com/spf13/cobra"
)

// git-wire subcommands: fetch single subfolders and track them for updates.

// wireBackend is the Fetcher the commands run against. Tests inject fakes
// through the fetchWith/updateWith/listWith helpers instead of mutating it.
var wireBackend wire.Fetcher

// initWireBackend builds the real backend once from the default store.
func initWireBackend() {
	if wireBackend != nil {
		return
	}

	store, err := wire.NewStore()
	if err != nil {
		panic(fmt.Sprintf("init wire store: %v", err))
	}

	wireBackend = wire.NewFetcher(store)
}

// addWireTargetFlags registers the target selection flags shared by
// fetch and update invocations. forceUsage documents the verb-specific
// meaning of --force.
func addWireTargetFlags(cmd *cobra.Command, forceUsage string) {
	cmd.Flags().StringP("target-path", "t", "",
		"Checkout directory (fetch defaults to ./<folder-name>, update defaults to the tracked folder)")
	cmd.Flags().Bool("force", false, forceUsage)
}

// addWireTargetNameFlag registers --target-name on fetch: a bare folder
// name for a folder that does not exist yet, created under the working
// directory. It is fetch-only and mutually exclusive with --target-path.
func addWireTargetNameFlag(cmd *cobra.Command) {
	cmd.Flags().StringP("target-name", "n", "",
		"New folder name to create under the working directory (cannot combine with --target-path)")
}

const (
	fetchForceUsage  = "Replace a non-empty target directory"
	updateForceUsage = "Discard local modifications and re-export upstream state"
)

// fetchWith runs the fetch flow against fetcher so tests can inject fakes.
func fetchWith(cmd *cobra.Command, args []string, fetcher wire.Fetcher) error {
	source, err := wire.ParseSourceURL(args[0])
	if err != nil {
		return wireError(err)
	}

	target, err := targetFromFlags(cmd, source)
	if err != nil {
		return err
	}

	force, _ := cmd.Flags().GetBool("force")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("working directory: %w", err)
	}

	summary, err := wire.Fetch(cmd.Context(),
		wire.FetchOptions{Fetcher: fetcher, Force: force, RegistryPath: wire.RegistryPath(cwd)}, source, target)
	if err != nil {
		return wireError(err)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), summary)

	return nil
}

// targetFromFlags resolves the destination: explicit --target-path wins,
// --target-name creates ./NAME for a name that does not exist yet,
// otherwise ./<subfolder-basename> under the working directory. Passing
// both flags fails fast as a usage error before any network use.
func targetFromFlags(cmd *cobra.Command, source wire.SourceRef) (string, error) {
	pathChanged := cmd.Flags().Changed("target-path")
	nameChanged := cmd.Flags().Changed("target-name")

	if pathChanged && nameChanged {
		return "", errors.New("--target-path and --target-name cannot be used together")
	}

	if nameChanged {
		name, err := cmd.Flags().GetString("target-name")
		if err != nil {
			return "", err
		}

		if err := validateTargetName(name); err != nil {
			return "", err
		}

		dest := "./" + name

		if _, err := os.Stat(dest); err == nil {
			return "", fmt.Errorf("target %s already exists: %w", dest, wire.ErrTargetNotEmpty)
		} else if !os.IsNotExist(err) {
			return "", fmt.Errorf("inspect target %s: %w", dest, err)
		}

		return dest, nil
	}

	if pathChanged {
		target, err := cmd.Flags().GetString("target-path")
		if err != nil {
			return "", err
		}

		if target == "" {
			return "", errors.New("empty --target-path")
		}

		return target, nil
	}

	return "./" + path.Base(source.Subpath), nil
}

// validateTargetName enforces the single-segment discipline for
// --target-name: a non-empty bare folder name with no separators, no dot
// elements, and no leading dash (mirroring ParseSourceURL segment rules).
func validateTargetName(name string) error {
	if name == "" {
		return errors.New("empty --target-name")
	}

	if name == "." || name == ".." {
		return fmt.Errorf("invalid --target-name %q: must name a new folder", name)
	}

	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("invalid --target-name %q: must not start with '-'", name)
	}

	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid --target-name %q: must be a single folder name", name)
	}

	return nil
}

// wireError appends the failure class guidance to an operational error,
// preserving the errors.Is chain for tests and callers.
func wireError(err error) error {
	return fmt.Errorf("%w\n%s", err, wire.NextAction(err))
}

var gitWireUpdateCmd = &cobra.Command{
	Use:   "update [path]",
	Short: "Update a tracked folder from its source",
	Args:  cobra.MaximumNArgs(1),
	Long: `Update a tracked folder from the source recorded at fetch time.

Resolves the tracked reference, reports "Already up to date" when nothing
changed, merges upstream changes when local edits do not conflict, and
refuses with the conflicting paths when they do (use --force to discard
local modifications).`,
	Example: `  git wire update ./auto_backup`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initWireBackend()

		return updateWith(cmd, args, wireBackend)
	},
}

// updateWith runs the update flow against fetcher so tests can inject fakes.
func updateWith(cmd *cobra.Command, args []string, fetcher wire.Fetcher) error {
	target, err := updateTarget(cmd, args)
	if err != nil {
		return err
	}

	force, _ := cmd.Flags().GetBool("force")

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("working directory: %w", err)
	}

	summary, err := wire.Update(cmd.Context(),
		wire.UpdateOptions{Fetcher: fetcher, Force: force, RegistryPath: wire.RegistryPath(cwd)}, target)
	if err != nil {
		return wireError(err)
	}

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), summary)

	return nil
}

// updateTarget resolves the checkout: explicit --target-path wins, then the
// positional path, then the single registry entry when the run-level
// registry holds exactly one. Zero entries report ErrNotACheckout;
// multiple entries fail as a usage error naming the candidates.
func updateTarget(cmd *cobra.Command, args []string) (string, error) {
	if cmd.Flags().Changed("target-path") {
		target, err := cmd.Flags().GetString("target-path")
		if err != nil {
			return "", err
		}

		if target == "" {
			return "", errors.New("empty --target-path")
		}

		return target, nil
	}

	if len(args) > 0 {
		return args[0], nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("working directory: %w", err)
	}

	reg, err := wire.LoadRegistry(wire.RegistryPath(cwd))
	if err != nil {
		return "", err
	}

	keys := reg.Keys()

	switch len(keys) {
	case 0:
		return "", fmt.Errorf("no tracked folders under %s: %w", cwd, wire.ErrNotACheckout)
	case 1:
		return keys[0], nil
	default:
		return "", fmt.Errorf("multiple tracked folders (%s): specify one with --target-path or a path argument",
			strings.Join(keys, ", "))
	}
}

var gitWireListCmd = &cobra.Command{
	Use:   "list [root]",
	Short: "List tracked folders and their sync state",
	Args:  cobra.MaximumNArgs(1),
	Long: `List folders fetched by git-wire under a directory, with the source
each came from and whether it is current, behind, diverged, or unreachable.

Unreachable sources are reported as rows, not failures.`,
	Example: `  git wire list ./modules`,
	RunE: func(cmd *cobra.Command, args []string) error {
		initWireBackend()

		return listWith(cmd, args, wireBackend)
	},
}

// listWith runs the list flow against fetcher so tests can inject fakes.
func listWith(cmd *cobra.Command, args []string, fetcher wire.Fetcher) error {
	root := "."

	if len(args) > 0 {
		root = args[0]
	}

	entries, err := wire.List(cmd.Context(), fetcher, root)
	if err != nil {
		return wireError(err)
	}

	if len(entries) == 0 {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No tracked folders under %s.\n", root)

		return nil
	}

	for _, e := range entries {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s  %s  %s  %s\n",
			e.Dir, e.Entry.Source().Display(), listSHA(e), e.State)
	}

	return nil
}

// listSHA renders the commit to show: the resolved remote SHA when known,
// otherwise the last recorded commit.
func listSHA(e wire.Entry) string {
	if e.RemoteSHA != "" {
		return wire.ShortSHA(e.RemoteSHA)
	}

	return wire.ShortSHA(e.Entry.ResolvedCommit)
}
