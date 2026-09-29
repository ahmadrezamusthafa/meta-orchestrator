package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Weight is the dynamic routing weight of one task type.
type Weight struct {
	TaskType      string             `json:"task_type"`
	TierWeights   map[string]float64 `json:"tier_weights"`   // tier → calibrated success probability
	PreferredTier string             `json:"preferred_tier"` // tier best-practice routing should use
	Runs          int                `json:"runs"`
	Reason        string             `json:"reason"`
	Locked        bool               `json:"locked"` // admin override: calibration never changes a locked weight
	LockedBy      string             `json:"locked_by,omitempty"`
	UpdatedAt     time.Time          `json:"updated_at"`
}

// WeightStore persists the routing weight table.
type WeightStore interface {
	Get(ctx context.Context, taskType string) (Weight, bool, error)
	Set(ctx context.Context, w Weight) error
	Delete(ctx context.Context, taskType string) error
	List(ctx context.Context) ([]Weight, error)
}

// MemoryWeightStore is an in-process WeightStore (tests, Redis-less dev).
type MemoryWeightStore struct {
	mu      sync.RWMutex
	weights map[string]Weight
}

// NewMemoryWeightStore creates an empty store.
func NewMemoryWeightStore() *MemoryWeightStore {
	return &MemoryWeightStore{weights: make(map[string]Weight)}
}

func (s *MemoryWeightStore) Get(ctx context.Context, taskType string) (Weight, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w, ok := s.weights[taskType]
	return w, ok, nil
}

func (s *MemoryWeightStore) Set(ctx context.Context, w Weight) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.weights[w.TaskType] = w
	return nil
}

func (s *MemoryWeightStore) Delete(ctx context.Context, taskType string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.weights, taskType)
	return nil
}

func (s *MemoryWeightStore) List(ctx context.Context) ([]Weight, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Weight, 0, len(s.weights))
	for _, w := range s.weights {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskType < out[j].TaskType })
	return out, nil
}

// DefaultRedisWeightsKey is the Redis hash holding the routing weight table.
const DefaultRedisWeightsKey = "orchestrator:router:weights"

// RedisWeightStore keeps the weight table in a Redis hash: field = task type, value = JSON weight.
type RedisWeightStore struct {
	client *redis.Client
	key    string
}

// NewRedisWeightStore creates a Redis-backed store.
func NewRedisWeightStore(client *redis.Client, key string) *RedisWeightStore {
	if key == "" {
		key = DefaultRedisWeightsKey
	}
	return &RedisWeightStore{client: client, key: key}
}

func (s *RedisWeightStore) Get(ctx context.Context, taskType string) (Weight, bool, error) {
	raw, err := s.client.HGet(ctx, s.key, taskType).Result()
	if err == redis.Nil {
		return Weight{}, false, nil
	}
	if err != nil {
		return Weight{}, false, fmt.Errorf("redis weights get: %w", err)
	}
	var w Weight
	if err := json.Unmarshal([]byte(raw), &w); err != nil {
		return Weight{}, false, fmt.Errorf("redis weights decode %s: %w", taskType, err)
	}
	return w, true, nil
}

func (s *RedisWeightStore) Set(ctx context.Context, w Weight) error {
	data, err := json.Marshal(w)
	if err != nil {
		return err
	}
	return s.client.HSet(ctx, s.key, w.TaskType, data).Err()
}

func (s *RedisWeightStore) Delete(ctx context.Context, taskType string) error {
	return s.client.HDel(ctx, s.key, taskType).Err()
}

func (s *RedisWeightStore) List(ctx context.Context) ([]Weight, error) {
	all, err := s.client.HGetAll(ctx, s.key).Result()
	if err != nil {
		return nil, fmt.Errorf("redis weights list: %w", err)
	}
	out := make([]Weight, 0, len(all))
	for _, raw := range all {
		var w Weight
		if err := json.Unmarshal([]byte(raw), &w); err != nil {
			continue
		}
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskType < out[j].TaskType })
	return out, nil
}
