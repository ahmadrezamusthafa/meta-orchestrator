package router

import (
	"fmt"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// BudgetTracker monitors and enforces cumulative token consumption per task.
type BudgetTracker struct {
	mu     sync.RWMutex
	usage  map[string]*types.TokenUsage
	budget map[string]int64
}

// NewBudgetTracker initializes a token budget tracker.
func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{
		usage:  make(map[string]*types.TokenUsage),
		budget: make(map[string]int64),
	}
}

// SetTaskBudget configures max token allowance for a task.
func (bt *BudgetTracker) SetTaskBudget(taskID string, maxTokens int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.budget[taskID] = maxTokens
	if _, exists := bt.usage[taskID]; !exists {
		bt.usage[taskID] = &types.TokenUsage{}
	}
}

// RecordUsage accumulates token usage and errors if budget is exceeded.
func (bt *BudgetTracker) RecordUsage(taskID string, promptTokens int64, completionTokens int64, costUSD float64) (*types.TokenUsage, error) {
	bt.mu.Lock()
	defer bt.mu.Unlock()

	u, exists := bt.usage[taskID]
	if !exists {
		u = &types.TokenUsage{}
		bt.usage[taskID] = u
	}

	u.PromptTokens += promptTokens
	u.CompletionTokens += completionTokens
	u.TotalTokens += (promptTokens + completionTokens)
	u.EstimatedCostUSD += costUSD

	max, hasMax := bt.budget[taskID]
	if hasMax && max > 0 && u.TotalTokens > max {
		return u, fmt.Errorf("task %s exceeded max token budget (%d > %d tokens)", taskID, u.TotalTokens, max)
	}

	cp := *u
	return &cp, nil
}

// GetUsage retrieves current accumulated token usage for a task.
func (bt *BudgetTracker) GetUsage(taskID string) types.TokenUsage {
	bt.mu.RLock()
	defer bt.mu.RUnlock()

	if u, exists := bt.usage[taskID]; exists {
		return *u
	}
	return types.TokenUsage{}
}
