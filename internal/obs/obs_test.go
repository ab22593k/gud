package obs

import (
	"testing"
	"time"
)

func TestTimer_DoneDoesNotPanic(t *testing.T) {
	t.Parallel()

	tm := Start("test_span")

	time.Sleep(2 * time.Millisecond)

	tm.Done("ok", true)

	if tm.Millis() < 0 {
		t.Fatalf("Millis=%d, want >=0", tm.Millis())
	}
}

func TestLogSizes_DoesNotPanic(t *testing.T) {
	t.Parallel()

	LogSizes(100, 200)
	LogSizes(0, 0)
}
