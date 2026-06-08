// Command git-message generates meaningful git commit messages using Google's Gemini API.
package main

import (
	"fmt"
	"os"

	msg "gud/cmd/git-message/core"
)

func main() {
	if err := msg.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
