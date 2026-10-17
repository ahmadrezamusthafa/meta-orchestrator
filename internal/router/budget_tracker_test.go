package router

import (
	"testing"
)

func TestBudgetTracker(t *testing.T) {
	bt := NewBudgetTracker()
	taskID := "task-budget-1"
	bt.SetTaskBudget(taskID, 1000)

	// Record 400 tokens -> OK
	u, err := bt.RecordUsage(taskID, 300, 100, 0.002)
	if err != nil {
		t.Fatalf("unexpected budget error: %v", err)
	}
	if u.TotalTokens != 400 {
		t.Errorf("expected 400 total tokens, got %d", u.TotalTokens)
	}

	// Record 500 tokens -> Total 900 -> OK
	u, err = bt.RecordUsage(taskID, 400, 100, 0.003)
	if err != nil {
		t.Fatalf("unexpected budget error: %v", err)
	}
	if u.TotalTokens != 900 {
		t.Errorf("expected 900 total tokens, got %d", u.TotalTokens)
	}

	// Record 200 tokens -> Total 1100 -> MUST ERROR (exceeds 1000 budget)
	_, err = bt.RecordUsage(taskID, 150, 50, 0.001)
	if err == nil {
		t.Fatalf("expected budget exceeded error")
	}
}

// The over-budget result must be a snapshot too: handing out the tracker's own record lets the
// caller mutate (and race on) state the tracker guards with its lock.
func TestBudgetTrackerOverBudgetReturnsCopy(t *testing.T) {
	bt := NewBudgetTracker()
	bt.SetTaskBudget("t", 10)
	u, err := bt.RecordUsage("t", 20, 0, 0)
	if err == nil || u == nil {
		t.Fatalf("expected over-budget usage and error, got %v %v", u, err)
	}
	u.TotalTokens = 0
	if got := bt.GetUsage("t").TotalTokens; got != 20 {
		t.Fatalf("tracker state changed through returned usage: %d", got)
	}
}
