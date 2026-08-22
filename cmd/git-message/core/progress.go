package core

import (
	"context"
	"fmt"
	"os"
	"time"
)

// showProgress displays an animated spinner on stderr while fn executes.
// It returns fn's result values. If ctx is cancelled before fn completes,
// the spinner stops immediately and ctx.Err() is returned. The worker
// goroutine running fn is notified via ctx but cannot be forcibly stopped;
// it silently discards its result to avoid leaking on cancellation.
func showProgress[T any](ctx context.Context, msg string, fn func() (T, error)) (T, error) {
	type result struct {
		val      T
		err      error
		panicVal any
	}

	resCh := make(chan result, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				select {
				case resCh <- result{panicVal: r}:
				case <-ctx.Done():
					// Context cancelled — nobody is listening.
				}
			}
		}()

		val, err := fn()
		select {
		case resCh <- result{val: val, err: err}:
		case <-ctx.Done():
			// Context cancelled — discard result silently.
		}
	}()

	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

	ticker := time.NewTicker(80 * time.Millisecond)
	defer ticker.Stop()

	// Write initial spinner immediately so the user sees feedback.
	fmt.Fprintf(os.Stderr, "\r%s %s", spinner[0], msg)

	i := 1

	for {
		select {
		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\r%s ✗ (cancelled)\n", msg)

			var zero T

			return zero, ctx.Err()

		case r := <-resCh:
			if r.panicVal != nil {
				fmt.Fprintf(os.Stderr, "\r%s ✗\n", msg)
				// Re-panic in the main goroutine so the caller sees it.
				panic(r.panicVal)
			}

			fmt.Fprintf(os.Stderr, "\r%s ✓\n", msg)

			return r.val, r.err

		case <-ticker.C:
			fmt.Fprintf(os.Stderr, "\r%s %s", spinner[i], msg)
			i = (i + 1) % len(spinner)
		}
	}
}
