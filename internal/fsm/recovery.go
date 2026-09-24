package fsm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
	"github.com/redis/go-redis/v9"
)

// TaskStateStore handles persisting and recovering task FSM states.
type TaskStateStore interface {
	SaveTask(ctx context.Context, task *types.Task) error
	GetTask(ctx context.Context, taskID string) (*types.Task, error)
	ListActiveTasks(ctx context.Context) ([]*types.Task, error)
}

// RedisTaskStateStore implements TaskStateStore backed by Redis.
type RedisTaskStateStore struct {
	client *redis.Client
}

// NewRedisTaskStateStore creates a store connecting to Redis.
func NewRedisTaskStateStore(client *redis.Client) *RedisTaskStateStore {
	return &RedisTaskStateStore{client: client}
}

func taskKey(taskID string) string {
	return fmt.Sprintf("orchestrator:task:%s:state", taskID)
}

const activeTasksSetKey = "orchestrator:tasks:active"

func (s *RedisTaskStateStore) SaveTask(ctx context.Context, task *types.Task) error {
	if s.client == nil {
		return fmt.Errorf("redis client is nil")
	}

	data, err := json.Marshal(task)
	if err != nil {
		return fmt.Errorf("failed to marshal task state: %w", err)
	}

	key := taskKey(task.ID)
	pipe := s.client.Pipeline()
	pipe.Set(ctx, key, data, 7*24*time.Hour)

	if task.State == types.TaskStateRunning ||
		task.State == types.TaskStatePending ||
		task.State == types.TaskStateWaitingGateApproval ||
		task.State == types.TaskStateBlockedFrustration {
		pipe.SAdd(ctx, activeTasksSetKey, task.ID)
	} else {
		pipe.SRem(ctx, activeTasksSetKey, task.ID)
	}

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to persist task state to redis: %w", err)
	}
	return nil
}

func (s *RedisTaskStateStore) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	if s.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	val, err := s.client.Get(ctx, taskKey(taskID)).Result()
	if err != nil {
		return nil, fmt.Errorf("task %s not found in redis: %w", taskID, err)
	}

	var task types.Task
	if err := json.Unmarshal([]byte(val), &task); err != nil {
		return nil, fmt.Errorf("failed to unmarshal task state: %w", err)
	}
	return &task, nil
}

func (s *RedisTaskStateStore) ListActiveTasks(ctx context.Context) ([]*types.Task, error) {
	if s.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}

	taskIDs, err := s.client.SMembers(ctx, activeTasksSetKey).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to read active tasks set: %w", err)
	}

	var active []*types.Task
	for _, id := range taskIDs {
		t, err := s.GetTask(ctx, id)
		if err == nil && t != nil {
			active = append(active, t)
		}
	}
	return active, nil
}

// MemoryTaskStateStore provides an in-memory implementation for testing or local running.
type MemoryTaskStateStore struct {
	mu    sync.RWMutex
	tasks map[string]*types.Task
}

// NewMemoryTaskStateStore creates an in-memory store.
func NewMemoryTaskStateStore() *MemoryTaskStateStore {
	return &MemoryTaskStateStore{
		tasks: make(map[string]*types.Task),
	}
}

func (m *MemoryTaskStateStore) SaveTask(ctx context.Context, task *types.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cp := *task
	m.tasks[task.ID] = &cp
	return nil
}

func (m *MemoryTaskStateStore) GetTask(ctx context.Context, taskID string) (*types.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	cp := *t
	return &cp, nil
}

func (m *MemoryTaskStateStore) ListActiveTasks(ctx context.Context) ([]*types.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*types.Task
	for _, t := range m.tasks {
		if t.State == types.TaskStateRunning ||
			t.State == types.TaskStatePending ||
			t.State == types.TaskStateWaitingGateApproval ||
			t.State == types.TaskStateBlockedFrustration {
			cp := *t
			active = append(active, &cp)
		}
	}
	return active, nil
}

// RecoverActiveTasks loads all active tasks to resume execution after process restart.
func RecoverActiveTasks(ctx context.Context, store TaskStateStore, reg *WorkflowRegistry) ([]*StateMachine, error) {
	start := time.Now()
	tasks, err := store.ListActiveTasks(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to recover active tasks: %w", err)
	}

	hydrator := NewSliceHydrator()
	var restoredFSMs []*StateMachine

	for _, task := range tasks {
		def, err := reg.Get(task.WorkflowID)
		if err != nil {
			continue
		}

		slicePlan, err := hydrator.HydrateSlice(def, task.ActiveSlice, nil)
		if err != nil {
			continue
		}

		fsm, err := NewStateMachine(task, def, slicePlan, func(t *types.Task) error {
			return store.SaveTask(context.Background(), t)
		}, nil)
		if err == nil {
			restoredFSMs = append(restoredFSMs, fsm)
		}
	}

	elapsed := time.Since(start)
	if elapsed > 5*time.Second {
		// Log warning if recovery took longer than target 5s RTO
		fmt.Printf("WARNING: Task recovery exceeded 5s RTO (took %v)\n", elapsed)
	}

	return restoredFSMs, nil
}
