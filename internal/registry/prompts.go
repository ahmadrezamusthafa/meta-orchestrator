package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// PromptSlots holds dynamic values substituted into prompt templates.
type PromptSlots struct {
	TaskID             string
	ActiveWorkflow     string
	CurrentStage       string
	SystemArchitecture string
	FailingTestTraces  string
	TargetASTSlice     string
	TaskSpec           string
	CustomContext      string
}

// PromptRegistry manages prompt templates across Project > System > Builtin tiers.
type PromptRegistry struct {
	mu            sync.RWMutex
	projectRoot   string
	systemDir     string
	builtinsDir   string
	cachedPrompts map[string]string
}

// NewPromptRegistry initializes a prompt registry.
func NewPromptRegistry(projectRoot string) *PromptRegistry {
	userHome, _ := os.UserHomeDir()
	return &PromptRegistry{
		projectRoot:   projectRoot,
		systemDir:     filepath.Join(userHome, ".config", "meta-orchestrator", "prompts"),
		builtinsDir:   "prompts",
		cachedPrompts: make(map[string]string),
	}
}

// LoadAndRender resolves the prompt template and substitutes slot placeholders.
func (r *PromptRegistry) LoadAndRender(templateName string, slots *PromptSlots) (string, error) {
	raw, err := r.ResolveTemplate(templateName)
	if err != nil {
		return "", err
	}

	return r.RenderTemplate(raw, slots), nil
}

// ResolveTemplate resolves template content with cascading priority:
// 1. Project Local: {projectRoot}/.sdlc/prompts/{templateName}
// 2. User System: ~/.config/meta-orchestrator/prompts/{templateName}
// 3. Built-in: prompts/{templateName}
func (r *PromptRegistry) ResolveTemplate(templateName string) (string, error) {
	// Normalize extension
	if !strings.HasSuffix(templateName, ".md") {
		templateName += ".md"
	}

	// 1. Project Local
	if r.projectRoot != "" {
		localPath := filepath.Join(r.projectRoot, ".sdlc", "prompts", templateName)
		if data, err := os.ReadFile(localPath); err == nil {
			return string(data), nil
		}
	}

	// 2. User System
	if r.systemDir != "" {
		sysPath := filepath.Join(r.systemDir, templateName)
		if data, err := os.ReadFile(sysPath); err == nil {
			return string(data), nil
		}
	}

	// 3. Built-in
	builtinPath := filepath.Join(r.builtinsDir, templateName)
	if data, err := os.ReadFile(builtinPath); err == nil {
		return string(data), nil
	}

	return "", fmt.Errorf("prompt template '%s' not found across local, system, or built-in paths", templateName)
}

// RenderTemplate substitutes {{placeholder}} variables.
func (r *PromptRegistry) RenderTemplate(template string, slots *PromptSlots) string {
	if slots == nil {
		return template
	}

	out := template
	out = strings.ReplaceAll(out, "{{task_id}}", slots.TaskID)
	out = strings.ReplaceAll(out, "{{active_workflow}}", slots.ActiveWorkflow)
	out = strings.ReplaceAll(out, "{{current_stage}}", slots.CurrentStage)
	out = strings.ReplaceAll(out, "{{system_architecture}}", slots.SystemArchitecture)
	out = strings.ReplaceAll(out, "{{failing_test_traces}}", slots.FailingTestTraces)
	out = strings.ReplaceAll(out, "{{target_ast_slice}}", slots.TargetASTSlice)
	out = strings.ReplaceAll(out, "{{task_spec}}", slots.TaskSpec)
	out = strings.ReplaceAll(out, "{{custom_context}}", slots.CustomContext)

	return out
}
