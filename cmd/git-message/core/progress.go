package core

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

// showProgress displays an animated spinner on errW while fn executes.
// Pass cmd.ErrOrStderr() so output follows command redirection (and a future
// --quiet flag can silence it with io.Discard); a nil writer falls back to
// os.Stderr. It returns fn's result values. If ctx is cancelled before fn
// completes, the spinner stops immediately and ctx.Err() is returned. The
// worker goroutine running fn is notified via ctx but cannot be forcibly
// stopped; it silently discards its result to avoid leaking on cancellation.
func showProgress[T any](ctx context.Context, errW io.Writer, msg string, fn func() (T, error)) (T, error) {
	type result struct {
		val      T
		err      error
		panicVal any
	}

	if errW == nil {
		errW = os.Stderr
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
	_, _ = fmt.Fprintf(errW, "\r%s %s", spinner[0], msg)

	i := 1

	for {
		select {
		case <-ctx.Done():
			_, _ = fmt.Fprintf(errW, "\r%s ✗ (cancelled)\n", msg)

			var zero T

			return zero, ctx.Err()

		case r := <-resCh:
			if r.panicVal != nil {
				_, _ = fmt.Fprintf(errW, "\r%s ✗\n", msg)

				var zero T

				return zero, fmt.Errorf("progress worker panicked: %v", r.panicVal)
			}

			_, _ = fmt.Fprintf(errW, "\r%s ✓\n", msg)

			return r.val, r.err

		case <-ticker.C:
			_, _ = fmt.Fprintf(errW, "\r%s %s", spinner[i], msg)
			i = (i + 1) % len(spinner)
		}
	}
}
