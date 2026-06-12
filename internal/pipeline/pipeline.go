// Package pipeline provides a worker pipeline pattern with context
// propagation and error handling for processing jobs sequentially.
package pipeline

import (
	"context"
	"fmt"
)

// Job is a unit of work for the pipeline.
type Job struct {
	ID     int
	Handle func(context.Context) error
}

// Run processes all jobs sequentially through a worker goroutine.
// Errors from job processing or context cancellation are returned.
func Run(ctx context.Context, jobs []Job) error {
	if len(jobs) == 0 {
		return nil
	}

	jobCh := make(chan Job, len(jobs))
	errCh := make(chan error, 1)

	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	go worker(ctx, jobCh, errCh)

	select {
	case err, ok := <-errCh:
		if !ok {
			return nil
		}

		return err
	case <-ctx.Done():
		return fmt.Errorf("pipeline cancelled: %w", ctx.Err())
	}
}

// worker reads jobs from ch and processes them until the channel is closed
// or an error occurs. The first error is sent to errCh.
func worker(ctx context.Context, jobs <-chan Job, errCh chan<- error) {
	defer close(errCh)
	for {
		select {
		case <-ctx.Done():
			errCh <- fmt.Errorf("worker cancelled: %w", ctx.Err())

			return
		case job, ok := <-jobs:
			if !ok {
				return
			}

			if err := job.Handle(ctx); err != nil {
				errCh <- fmt.Errorf("process job %d: %w", job.ID, err)

				return
			}
		}
	}
}
