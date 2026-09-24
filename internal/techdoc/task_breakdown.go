package techdoc

import (
	"fmt"
	"strings"
)

// AtomicTaskItem defines a single implementation step.
type AtomicTaskItem struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	AssignedRole string   `json:"assigned_role"`
	TargetRepo   string   `json:"target_repo"`
	Dependencies []string `json:"dependencies"`
}

// TaskBreakdownPlanner decomposes approved RFCs into sequenced atomic tasks.
type TaskBreakdownPlanner struct{}

// NewTaskBreakdownPlanner creates a planner.
func NewTaskBreakdownPlanner() *TaskBreakdownPlanner {
	return &TaskBreakdownPlanner{}
}

// GenerateTaskPlanMarkdown builds TASK_PLAN.md.
func (p *TaskBreakdownPlanner) GenerateTaskPlanMarkdown(featureTitle string, tasks []AtomicTaskItem) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Master Task Breakdown Plan: %s\n\n", featureTitle))
	b.WriteString("## Atomic Implementation Sequence\n\n")

	for i, t := range tasks {
		deps := "None"
		if len(t.Dependencies) > 0 {
			deps = strings.Join(t.Dependencies, ", ")
		}
		b.WriteString(fmt.Sprintf("### %d. %s: %s\n", i+1, t.ID, t.Title))
		b.WriteString(fmt.Sprintf("- **Role:** `%s`\n", t.AssignedRole))
		b.WriteString(fmt.Sprintf("- **Repo:** `%s`\n", t.TargetRepo))
		b.WriteString(fmt.Sprintf("- **Prerequisites:** %s\n\n", deps))
	}

	return b.String()
}
