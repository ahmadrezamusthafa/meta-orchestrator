package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"github.com/redis/go-redis/v9"
)

// TaskQueue abstracts task queuing with Redis BullMQ-compatible priority semantics.
type TaskQueue interface {
	Enqueue(ctx context.Context, task *types.Task) error
	Dequeue(ctx context.Context, timeout time.Duration) (*types.Task, error)
	Size(ctx context.Context) (int64, error)
}

// RedisTaskQueue implements TaskQueue backed by Redis lists and sets.
type RedisTaskQueue struct {
	client    *redis.Client
	queueKey  string
	activeKey string
}

// NewRedisTaskQueue creates a Redis-backed queue.
func NewRedisTaskQueue(client *redis.Client, queueName string) *RedisTaskQueue {
	if queueName == "" {
		queueName = "default"
	}
	return &RedisTaskQueue{
		client:    client,
		queueKey:  fmt.Sprintf("orchestrator:queue:%s:tasks", queueName),
		activeKey: fmt.Sprintf("orchestrator:queue:%s:processing", queueName),
	}
}

// Enqueue adds a task to the tail of the Redis queue.
func (q *RedisTaskQueue) Enqueue(ctx context.Context, task *types.Task) error {
	if q.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task for queue: %w", err)
	}

	return q.client.LPush(ctx, q.queueKey, data).Err()
}

// Dequeue pops a task from the head of the Redis queue using blocking RPOPLPUSH.
func (q *RedisTaskQueue) Dequeue(ctx context.Context, timeout time.Duration) (*types.Task, error) {
	if q.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	res, err := q.client.BRPopLPush(ctx, q.queueKey, q.activeKey, timeout).Result()
	if err != nil {
		return nil, err
	}

	var task types.Task
	if err := json.Unmarshal([]byte(res), &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal dequeued task: %w", err)
	}

	return &task, nil
}

// Acknowledge removes a completed task from the processing queue.
func (q *RedisTaskQueue) Acknowledge(ctx context.Context, task *types.Task) error {
	if q.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	return q.client.LRem(ctx, q.activeKey, 1, data).Err()
}

// Size returns total pending items in the queue.
func (q *RedisTaskQueue) Size(ctx context.Context) (int64, error) {
	if q.client == nil {
		return 0, fmt.Errorf("redis client is nil")
	}
	return q.client.LLen(ctx, q.queueKey).Result()
}
