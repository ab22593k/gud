// Command git-wire fetches single subfolders from hosted repositories,
// tracks them for later updates, and reports their sync state. The binary
// is built from cmd/git-wire and named git-wire, so it lands on PATH as
// `git wire` (git runs any git-* executable as `git <name>`).
package main

import (
	"fmt"
	"os"

	"gud/cmd/git-wire/core"
)

func main() {
	if err := core.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
