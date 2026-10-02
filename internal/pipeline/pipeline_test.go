package pipeline

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

// recordJob returns a handler that appends n to order when run. It takes a
// pointer so each job in a table can mutate the caller's slice.
func recordJob(order *[]int, n int) func(context.Context) error {
	return func(context.Context) error {
		*order = append(*order, n)

		return nil
	}
}

func TestRun_Success(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	var order []int

	jobs := []Job{
		{ID: 1, Handle: recordJob(&order, 1)},
		{ID: 2, Handle: recordJob(&order, 2)},
		{ID: 3, Handle: recordJob(&order, 3)},
	}

	if err := Run(ctx, jobs); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	if want := []int{1, 2, 3}; !slices.Equal(order, want) {
		t.Errorf("execution order = %v, want %v", order, want)
	}
}

func TestRun_EmptyJobs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if err := Run(ctx, nil); err != nil {
		t.Errorf("Run(nil) error = %v, want nil", err)
	}

	if err := Run(ctx, []Job{}); err != nil {
		t.Errorf("Run([]Job{}) error = %v, want nil", err)
	}
}

func TestRun_JobError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	want := "process failed"
	erroringJob := Job{
		ID:     1,
		Handle: func(_ context.Context) error { return errors.New(want) },
	}

	err := Run(ctx, []Job{erroringJob})
	if err == nil {
		t.Fatal("Run() error = nil, want non-nil")
	}

	if !strings.Contains(err.Error(), want) {
		t.Errorf("Run() error = %q, want to contain %q", err, want)
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	jobs := []Job{
		{ID: 1, Handle: func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		}},
	}

	err := Run(ctx, jobs)
	if err == nil {
		t.Fatal("Run() error = nil, want context.Canceled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRun_CancellationMidway(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	jobs := []Job{
		{ID: 1, Handle: func(_ context.Context) error {
			cancel()

			return nil
		}},
		{ID: 2, Handle: func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		}},
	}

	err := Run(ctx, jobs)
	if err == nil {
		t.Fatal("Run() error = nil, want context.Canceled")
	}

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Run() error = %v, want context.Canceled", err)
	}
}

func TestRun_Timeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()

	time.Sleep(time.Millisecond)

	jobs := []Job{
		{ID: 1, Handle: func(ctx context.Context) error {
			<-ctx.Done()

			return ctx.Err()
		}},
	}

	err := Run(ctx, jobs)
	if err == nil {
		t.Fatal("Run() error = nil, want deadline exceeded")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Run() error = %v, want context.DeadlineExceeded", err)
	}
}
