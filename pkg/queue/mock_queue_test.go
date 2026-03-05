package queue_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/afterdarksys/apiproxyd/pkg/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockQueue provides a simple in-memory queue implementation for testing
type MockQueue struct {
	tasks    []string
	handlers map[string]queue.TaskHandler
}

func NewMockQueue() *MockQueue {
	return &MockQueue{
		tasks:    make([]string, 0),
		handlers: make(map[string]queue.TaskHandler),
	}
}

func (m *MockQueue) Enqueue(ctx context.Context, taskName string, payload interface{}, opts ...queue.EnqueueOption) error {
	m.tasks = append(m.tasks, taskName)

	// Simply execute it immediately if a handler is registered (for testing purposes)
	if handler, ok := m.handlers[taskName]; ok {
		var bytes []byte
		var err error
		if p, isBytes := payload.([]byte); isBytes {
			bytes = p
		} else {
			bytes, err = json.Marshal(payload)
			if err != nil {
				return err
			}
		}
		return handler(ctx, bytes)
	}

	return nil
}

func (m *MockQueue) Start(ctx context.Context) error {
	return nil
}

func (m *MockQueue) Stop() error {
	return nil
}

func (m *MockQueue) RegisterWorker(taskName string, handler queue.TaskHandler) error {
	m.handlers[taskName] = handler
	return nil
}

// TestQueueIntegration verifies the generic interface behavior and
// that registering workers and enqueuing tasks behaves correctly.
func TestQueueIntegration(t *testing.T) {
	q := NewMockQueue()

	processed := false
	err := q.RegisterWorker("warm_cache", func(ctx context.Context, payload []byte) error {
		var data map[string]string
		if err := json.Unmarshal(payload, &data); err != nil {
			return err
		}

		assert.Equal(t, "foo", data["key"])
		processed = true
		return nil
	})
	require.NoError(t, err)

	// Enqueue a task
	err = q.Enqueue(context.Background(), "warm_cache", map[string]string{"key": "foo"})
	require.NoError(t, err)

	assert.Len(t, q.tasks, 1)
	assert.Equal(t, "warm_cache", q.tasks[0])
	assert.True(t, processed, "The task should have been processed by the mocked handler")
}
