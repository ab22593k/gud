package core

import (
	"github.com/spf13/cobra"
)

const versionCmdName = "version"

var versionCmd = &cobra.Command{
	Use:   versionCmdName,
	Short: "Print the version number",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cmd.Printf("gud version %s\n", version)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
