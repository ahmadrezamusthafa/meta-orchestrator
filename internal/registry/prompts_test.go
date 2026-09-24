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
