package task

import (
	"context"

	"golang.org/x/sync/errgroup"
)

// OnSuccess executes g() after f() returns nil.
func OnSuccess(f func() error, g func() error) func() error {
	return func() error {
		if err := f(); err != nil {
			return err
		}
		return g()
	}
}

// Run executes a list of tasks in parallel, returns the first error encountered or nil if all tasks pass.
func Run(ctx context.Context, tasks ...func() error) error {
	eg, ctx := errgroup.WithContext(ctx)
	for _, task := range tasks {
		task := task
		eg.Go(func() error {
			done := make(chan error, 1)
			go func() { done <- task() }()
			select {
			case err := <-done:
				return err
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}
	return eg.Wait()
}
