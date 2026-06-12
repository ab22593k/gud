package pipeline

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRun_Success(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var order []int
	jobs := []Job{
		{ID: 1, Handle: func(_ context.Context) error { order = append(order, 1); return nil }}, //nolint:nlreturn
		{ID: 2, Handle: func(_ context.Context) error { order = append(order, 2); return nil }}, //nolint:nlreturn
		{ID: 3, Handle: func(_ context.Context) error { order = append(order, 3); return nil }}, //nolint:nlreturn
	}
	if err := Run(ctx, jobs); err != nil {
		t.Errorf("Run() error = %v, want nil", err)
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
