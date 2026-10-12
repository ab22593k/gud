package core

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestIsTerminal_DevNullIsNotTerminal is the regression lock for the hook
// hang: /dev/null is a character device, so the old ModeCharDevice check
// classified hook stdin from /dev/null as a terminal and launched the
// AltScreen TUI, hanging the commit.
func TestIsTerminal_DevNullIsNotTerminal(t *testing.T) {
	t.Parallel()

	f, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("no /dev/null: %v", err)
	}

	defer func() { _ = f.Close() }()

	if isTerminal(f) {
		t.Error("isTerminal(/dev/null) = true, want false")
	}
}

func TestIsTerminal_PipesAreNotTerminals(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	if isTerminal(r) {
		t.Error("isTerminal(pipe reader) = true, want false")
	}

	if isTerminal(w) {
		t.Error("isTerminal(pipe writer) = true, want false")
	}
}

func TestIsTerminal_NilAndRegularFileAreNotTerminals(t *testing.T) {
	t.Parallel()

	if isTerminal(nil) {
		t.Error("isTerminal(nil) = true, want false")
	}

	f, err := os.CreateTemp(t.TempDir(), "gud-terminal-*")
	if err != nil {
		t.Fatalf("temp file: %v", err)
	}

	defer func() { _ = f.Close() }()
	defer func() { _ = os.Remove(f.Name()) }()

	if isTerminal(f) {
		t.Error("isTerminal(regular file) = true, want false")
	}
}

func TestIsInteractive_RequiresTerminalStdinAndStdout(t *testing.T) {
	t.Parallel()

	devNull, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("no /dev/null: %v", err)
	}

	defer func() { _ = devNull.Close() }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	cases := []struct {
		name string
		in   io.Reader
		out  io.Writer
	}{
		{name: "non-file streams", in: strings.NewReader("y\n"), out: &bytes.Buffer{}},
		{name: "devnull stdin, buffer stdout", in: devNull, out: &bytes.Buffer{}},
		{name: "buffer stdin, devnull stdout", in: strings.NewReader("y\n"), out: devNull},
		{name: "piped stdin and stdout", in: r, out: w},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if isInteractive(tc.in, tc.out) {
				t.Errorf("isInteractive(%s) = true, want false", tc.name)
			}
		})
	}
}

// TestReviewMessage_DevNullFallsBackToPrompt guards the hook hang end to end:
// stdin from /dev/null must take the plain-text path, which sees EOF and
// aborts, instead of launching the TUI. The timeout turns a hang regression
// into a failure instead of wedging the suite.
func TestReviewMessage_DevNullFallsBackToPrompt(t *testing.T) {
	t.Parallel()

	devNull, err := os.Open("/dev/null")
	if err != nil {
		t.Skipf("no /dev/null: %v", err)
	}

	defer func() { _ = devNull.Close() }()

	cmd := &cobra.Command{}
	cmd.SetIn(devNull)

	var out bytes.Buffer

	cmd.SetOut(&out)

	scanner := bufio.NewScanner(cmd.InOrStdin())

	type result struct {
		action string
		edited string
	}

	done := make(chan result, 1)

	go func() {
		action, edited := reviewMessage(cmd, scanner, &out, "feat: test", 72)
		done <- result{action: action, edited: edited}
	}()

	select {
	case got := <-done:
		if got.action != actionAbort {
			t.Errorf("reviewMessage(/dev/null) action = %q, want %q", got.action, actionAbort)
		}

		if got.edited != "" {
			t.Errorf("reviewMessage(/dev/null) edited = %q, want empty", got.edited)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("reviewMessage hung with /dev/null stdin; TUI must not launch without a terminal")
	}
}
