package techdoc

import (
	"strings"
	"testing"
)

func TestRFCGeneratorAndTaskBreakdown(t *testing.T) {
	rfcGen := NewRFCGenerator()
	planner := NewTaskBreakdownPlanner()

	// 1. Generate RFC
	spec := &RFCContractSpecification{
		FeatureTitle:    "Order Checkout Flow",
		TargetRepos:     []string{"frontend-portal", "backend-api"},
		DatabaseChanges: []string{"CREATE TABLE orders (id UUID PRIMARY KEY, user_id UUID, amount NUMERIC);"},
		APIEndpoints:    []string{"POST /api/v1/orders"},
		ImpactedClasses: []string{"OrderController", "PaymentService"},
	}

	rfcDoc := rfcGen.GenerateRFC(spec)
	if !strings.Contains(rfcDoc, "# Technical Design Document & RFC: Order Checkout Flow") {
		t.Errorf("missing RFC title")
	}
	if !strings.Contains(rfcDoc, "POST /api/v1/orders") {
		t.Errorf("missing API endpoint in RFC")
	}
	if !strings.Contains(rfcDoc, "CREATE TABLE orders") {
		t.Errorf("missing database migration in RFC")
	}

	// 2. Generate Task Plan
	tasks := []AtomicTaskItem{
		{
			ID:           "TASK-1.1",
			Title:        "Implement DB migration and Order model",
			AssignedRole: "lead_developer",
			TargetRepo:   "backend-api",
		},
		{
			ID:           "TASK-1.2",
			Title:        "Create Vue Checkout View and form validation",
			AssignedRole: "vue_frontend_engineer",
			TargetRepo:   "frontend-portal",
			Dependencies: []string{"TASK-1.1"},
		},
	}

	planDoc := planner.GenerateTaskPlanMarkdown("Order Checkout Flow", tasks)
	if !strings.Contains(planDoc, "TASK-1.1: Implement DB migration") {
		t.Errorf("missing task 1.1 in plan")
	}
	if !strings.Contains(planDoc, "TASK-1.2") {
		t.Errorf("missing task 1.2 in plan")
	}
}
