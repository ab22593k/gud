package main

import (
	"fmt"
	"os"

	cli "gud/cmd/message/core"
)

func main() {
	if err := cli.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
