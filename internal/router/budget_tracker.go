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

// SetTaskBudget starts a budget of maxTokens for the task's next stage run: usage recorded from
// here on is counted against it, so earlier stages do not use up a later stage's budget.
func (bt *BudgetTracker) SetTaskBudget(taskID string, maxTokens int64) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	bt.budget[taskID] = maxTokens
	bt.usage[taskID] = &types.TokenUsage{}
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

	cp := *u
	max, hasMax := bt.budget[taskID]
	if hasMax && max > 0 && u.TotalTokens > max {
		return &cp, fmt.Errorf("task %s exceeded max token budget (%d > %d tokens)", taskID, u.TotalTokens, max)
	}
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
