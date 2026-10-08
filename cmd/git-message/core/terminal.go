package core

import "os"

// isTerminal reports whether f is a character device (terminal).
//
// It gates the interactive paths: a TUI cannot render into a pipe, and a
// prepare-commit-msg hook that tried to open one would hang the commit.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}

	return fi.Mode()&os.ModeCharDevice != 0
}
