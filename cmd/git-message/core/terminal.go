package core

import (
	"io"
	"os"

	"golang.org/x/term"
)

// isTerminal reports whether f is a terminal.
//
// It gates the interactive paths: a TUI cannot render into a pipe, and a
// prepare-commit-msg hook that tried to open one would hang the commit.
//
// A ModeCharDevice check is insufficient: /dev/null is a character device but
// not a terminal, so it misclassified hook stdin from /dev/null and launched
// the AltScreen TUI. term.IsTerminal issues the TCGETS ioctl instead.
func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}

	return term.IsTerminal(int(f.Fd()))
}

// isInteractive reports whether the full-screen review TUI can run: both
// stdin and stdout must be terminals. Checking stdin alone is insufficient —
// with stdin on a tty but stdout piped, AltScreen sequences would be written
// into the pipe while input blocks.
func isInteractive(in io.Reader, out io.Writer) bool {
	inFile, ok := in.(*os.File)
	if !ok || !isTerminal(inFile) {
		return false
	}

	outFile, ok := out.(*os.File)
	if !ok || !isTerminal(outFile) {
		return false
	}

	return true
}
