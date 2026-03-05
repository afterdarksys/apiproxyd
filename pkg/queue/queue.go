package queue

import (
	"context"
	"time"
)

// TaskQueue is a pluggable interface for distributed background processing.
// Support can be backed by River (Postgres), Asynq (Redis), or an in-memory mock.
type TaskQueue interface {
	// Enqueue adds a job to the queue.
	// The payload is JSON serializable data representing arguments.
	Enqueue(ctx context.Context, taskName string, payload interface{}, opts ...EnqueueOption) error

	// Start begins processing the queue with the registered workers.
	Start(ctx context.Context) error

	// Stop halts the processing of new jobs gracefully.
	Stop() error

	// RegisterWorker adds a handler for a specific task type.
	RegisterWorker(taskName string, handler TaskHandler) error
}

// TaskHandler is the function signature for processing a queue item.
type TaskHandler func(ctx context.Context, payload []byte) error

// EnqueueOptions configure how a job is submitted.
type EnqueueOptions struct {
	Queue    string
	MaxRetry int
	Delay    time.Duration
}

type EnqueueOption func(*EnqueueOptions)

func WithQueue(q string) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.Queue = q
	}
}

func WithMaxRetry(r int) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.MaxRetry = r
	}
}

func WithDelay(d time.Duration) EnqueueOption {
	return func(o *EnqueueOptions) {
		o.Delay = d
	}
}
