package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptRegistryResolutionAndSlotFilling(t *testing.T) {
	tmpProject := t.TempDir()
	reg := NewPromptRegistry(tmpProject)
	reg.builtinsDir = filepath.Join("..", "..", "prompts")

	// 1. Resolve built-in base system prompt
	rendered, err := reg.LoadAndRender("base_system.md", &PromptSlots{
		TaskID:         "task-prompt-1",
		ActiveWorkflow: "general-ai-sdlc",
		CurrentStage:   "INTAKE_PRD",
		CustomContext:  "Extra user requirements here",
	})
	if err != nil {
		t.Fatalf("failed to render built-in prompt: %v", err)
	}

	if !strings.Contains(rendered, "Task ID: task-prompt-1") {
		t.Errorf("slot {{task_id}} not substituted properly")
	}
	if !strings.Contains(rendered, "Active Workflow: general-ai-sdlc") {
		t.Errorf("slot {{active_workflow}} not substituted properly")
	}
	if !strings.Contains(rendered, "Extra user requirements here") {
		t.Errorf("slot {{custom_context}} not substituted properly")
	}

	// 2. Project Local Override
	localDir := filepath.Join(tmpProject, ".sdlc", "prompts")
	_ = os.MkdirAll(localDir, 0755)
	overrideContent := `Project Custom Developer Prompt: {{task_id}} on {{current_stage}}`
	_ = os.WriteFile(filepath.Join(localDir, "developer_v1.md"), []byte(overrideContent), 0644)

	overridden, err := reg.LoadAndRender("developer_v1.md", &PromptSlots{
		TaskID:       "task-override-99",
		CurrentStage: "IMPLEMENTATION_GREEN",
	})
	if err != nil {
		t.Fatalf("failed to render overridden prompt: %v", err)
	}

	if !strings.Contains(overridden, "Project Custom Developer Prompt: task-override-99 on IMPLEMENTATION_GREEN") {
		t.Errorf("project override did not take precedence: %s", overridden)
	}
}

func TestBillingPromptsRealDirectory(t *testing.T) {
	billingPromptsDir := "/Users/rezamekari/Projects/go/src/bitbucket.org/mid-kelola-indonesia/billing/.github/prompts"
	if _, err := os.Stat(billingPromptsDir); os.IsNotExist(err) {
		t.Skipf("Billing prompts directory not found at %s, skipping live test", billingPromptsDir)
	}

	reg := NewPromptRegistry("")
	templates, err := reg.CheckDirectoryCompatibility(billingPromptsDir)
	if err != nil {
		t.Fatalf("Failed to check compatibility of billing prompts: %v", err)
	}

	if len(templates) < 15 {
		t.Errorf("Expected at least 15 prompt templates, found %d", len(templates))
	}

	foundPRD := false
	foundTechDoc := false
	foundBreakdown := false

	for _, tmpl := range templates {
		if !tmpl.Compatible {
			t.Errorf("Template %s marked as not compatible", tmpl.Name)
		}
		if strings.Contains(tmpl.ID, "prd-to-jira-user-story") {
			foundPRD = true
			if tmpl.Name == "" {
				t.Errorf("Template %s has empty Name", tmpl.ID)
			}
		}
		if strings.Contains(tmpl.ID, "generate-technical-document") {
			foundTechDoc = true
		}
		if strings.Contains(tmpl.ID, "task-breakdown") {
			foundBreakdown = true
		}
	}

	if !foundPRD {
		t.Errorf("prd-to-jira-user-story template not discovered")
	}
	if !foundTechDoc {
		t.Errorf("generate-technical-document template not discovered")
	}
	if !foundBreakdown {
		t.Errorf("task-breakdown template not discovered")
	}

	// Test registration
	source, registeredTemplates, err := reg.RegisterSource("Billing Prompts", billingPromptsDir)
	if err != nil {
		t.Fatalf("Failed to register billing prompts source: %v", err)
	}
	if source.TemplateCount != len(templates) {
		t.Errorf("Source template count mismatch: %d vs %d", source.TemplateCount, len(templates))
	}
	if len(registeredTemplates) != len(templates) {
		t.Errorf("Registered templates count mismatch")
	}

	// Test rendering of registered template
	rendered, err := reg.ResolveTemplate("prd-to-jira-user-story")
	if err != nil {
		t.Fatalf("Failed to resolve prd-to-jira-user-story from registered source: %v", err)
	}
	if !strings.Contains(rendered, "PRD to Jira User Story") {
		t.Errorf("Resolved template does not contain expected header")
	}

	// Test custom parameter substitution
	customRendered := reg.RenderCustom(rendered, map[string]string{
		"parent_issue_key": "MIB-1234",
	})
	if !strings.Contains(customRendered, "MIB-1234") && !strings.Contains(rendered, "MIB-9685") {
		t.Errorf("Rendering failed to produce expected content")
	}
}

