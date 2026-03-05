package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
)

// AsynqQueue implements TaskQueue using Redis with Asynq.
type AsynqQueue struct {
	client *asynq.Client
	server *asynq.Server
	mux    *asynq.ServeMux
}

func NewAsynqQueue(redisAddr string) (*AsynqQueue, error) {
	client := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})

	// Create a worker server
	server := asynq.NewServer(
		asynq.RedisClientOpt{Addr: redisAddr},
		asynq.Config{
			Concurrency: 10,
			Queues: map[string]int{
				"critical": 6,
				"default":  3,
				"low":      1,
			},
		},
	)

	mux := asynq.NewServeMux()

	return &AsynqQueue{
		client: client,
		server: server,
		mux:    mux,
	}, nil
}

func (q *AsynqQueue) RegisterWorker(taskName string, handler TaskHandler) error {
	q.mux.HandleFunc(taskName, func(ctx context.Context, t *asynq.Task) error {
		return handler(ctx, t.Payload())
	})
	return nil
}

func (q *AsynqQueue) Enqueue(ctx context.Context, taskName string, payload interface{}, opts ...EnqueueOption) error {
	config := &EnqueueOptions{
		Queue:    "default",
		MaxRetry: 5,
		Delay:    0,
	}
	for _, opt := range opts {
		opt(config)
	}

	rawPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(taskName, rawPayload, asynq.MaxRetry(config.MaxRetry), asynq.Queue(config.Queue))

	var asynqOpts []asynq.Option
	if config.Delay > 0 {
		asynqOpts = append(asynqOpts, asynq.ProcessIn(config.Delay))
	}

	_, err = q.client.EnqueueContext(ctx, task, asynqOpts...)
	return err
}

func (q *AsynqQueue) Start(ctx context.Context) error {
	// asynq Start is non-blocking
	return q.server.Start(q.mux)
}

func (q *AsynqQueue) Stop() error {
	q.server.Shutdown()
	return q.client.Close()
}
