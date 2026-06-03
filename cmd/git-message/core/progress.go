package core

import (
	"fmt"
	"os"
	"time"
)

// showProgress displays an animated spinner on stderr while fn executes.
// It returns fn's result values.
func showProgress[T any](msg string, fn func() (T, error)) (T, error) {
	type result struct {
		val T
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		val, err := fn()
		resCh <- result{val, err}
	}()

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case r := <-resCh:
			fmt.Fprintf(os.Stderr, "\r%s ✓\n", msg)
			return r.val, r.err
		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\r%s %s", spinner[i], msg)
			i = (i + 1) % len(spinner)
		}
	}
}
