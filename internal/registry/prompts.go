package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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

// PromptItemDTO represents an adapted prompt template specification.
type PromptItemDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Source         string   `json:"source"` // "PROJECT_LOCAL", "GITHUB_PROMPTS", "USER_SYSTEM", "CUSTOM_DIR", "BUILTIN"
	SourcePath     string   `json:"source_path,omitempty"`
	Role           string   `json:"role,omitempty"`
	Description    string   `json:"description,omitempty"`
	Variables      []string `json:"variables"`
	RawContent     string   `json:"raw_content,omitempty"`
	SystemOverride bool     `json:"system_override"`
	Compatible     bool     `json:"compatible"`
}

// PromptSource represents an external registered prompt directory.
type PromptSource struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
	TemplateCount int    `json:"template_count"`
	AddedAt       string `json:"added_at"`
}

type promptsConfigFile struct {
	RegisteredSources []PromptSource `json:"registered_sources"`
}

// PromptRegistry manages prompt templates across Project > GitHub > Custom > System > Builtin tiers.
type PromptRegistry struct {
	mu            sync.RWMutex
	projectRoot   string
	systemDir     string
	builtinsDir   string
	configPath    string
	customSources []PromptSource
	cachedPrompts map[string]string
}

// NewPromptRegistry initializes a prompt registry with cascading discovery.
func NewPromptRegistry(projectRoot string) *PromptRegistry {
	userHome, _ := os.UserHomeDir()
	configPath := ""
	if projectRoot != "" {
		configPath = filepath.Join(projectRoot, ".sdlc", "prompts_config.json")
	} else if userHome != "" {
		configPath = filepath.Join(userHome, ".config", "meta-orchestrator", "prompts_config.json")
	}

	reg := &PromptRegistry{
		projectRoot:   projectRoot,
		systemDir:     filepath.Join(userHome, ".config", "meta-orchestrator", "prompts"),
		builtinsDir:   "prompts",
		configPath:    configPath,
		customSources: make([]PromptSource, 0),
		cachedPrompts: make(map[string]string),
	}
	reg.loadConfig()
	return reg
}

func (r *PromptRegistry) loadConfig() {
	if r.configPath == "" {
		return
	}
	bytes, err := os.ReadFile(r.configPath)
	if err != nil {
		return
	}
	var data promptsConfigFile
	if err := json.Unmarshal(bytes, &data); err == nil && data.RegisteredSources != nil {
		r.customSources = data.RegisteredSources
	}
}

func (r *PromptRegistry) saveConfigLocked() error {
	if r.configPath == "" {
		return nil
	}
	dir := filepath.Dir(r.configPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data := promptsConfigFile{RegisteredSources: r.customSources}
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.configPath, bytes, 0644)
}

// ListSources returns registered custom prompt directory sources.
func (r *PromptRegistry) ListSources() []PromptSource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]PromptSource, len(r.customSources))
	copy(res, r.customSources)
	return res
}

// RegisterSource registers a new prompt template directory.
func (r *PromptRegistry) RegisterSource(name, dirPath string) (*PromptSource, []PromptItemDTO, error) {
	stat, err := os.Stat(dirPath)
	if err != nil || !stat.IsDir() {
		return nil, nil, fmt.Errorf("directory not found or inaccessible: %s", dirPath)
	}

	templates, err := r.CheckDirectoryCompatibility(dirPath)
	if err != nil {
		return nil, nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	sourceID := fmt.Sprintf("psrc-%d", time.Now().UnixNano())
	if name == "" {
		name = filepath.Base(dirPath)
	}

	src := PromptSource{
		ID:            sourceID,
		Name:          name,
		Path:          dirPath,
		TemplateCount: len(templates),
		AddedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	// Update existing if same path
	found := false
	for i, existing := range r.customSources {
		if existing.Path == dirPath {
			r.customSources[i] = src
			found = true
			break
		}
	}
	if !found {
		r.customSources = append(r.customSources, src)
	}
	_ = r.saveConfigLocked()

	return &src, templates, nil
}

// UnregisterSource unregisters a custom prompt source.
func (r *PromptRegistry) UnregisterSource(sourceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	idx := -1
	for i, s := range r.customSources {
		if s.ID == sourceID || s.Path == sourceID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("prompt source '%s' not found", sourceID)
	}

	r.customSources = append(r.customSources[:idx], r.customSources[idx+1:]...)
	return r.saveConfigLocked()
}

// CheckDirectoryCompatibility audits any directory on disk for prompt templates.
func (r *PromptRegistry) CheckDirectoryCompatibility(dirPath string) ([]PromptItemDTO, error) {
	stat, err := os.Stat(dirPath)
	if err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("invalid directory: %s", dirPath)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var results []PromptItemDTO
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".prompt.md") || strings.HasSuffix(name, ".md") {
			fullPath := filepath.Join(dirPath, name)
			if item, err := r.parsePromptFile(fullPath, "CUSTOM_DIR", true); err == nil && item != nil {
				results = append(results, *item)
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no prompt templates (*.prompt.md, *.md) found in %s", dirPath)
	}

	return results, nil
}

// ListPromptTemplates returns all discovered prompt templates across all tiers.
func (r *PromptRegistry) ListPromptTemplates() []PromptItemDTO {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []PromptItemDTO
	seen := make(map[string]bool)

	addItemsFromDir := func(dirPath string, sourceName string, override bool) {
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".prompt.md") || strings.HasSuffix(name, ".md") {
				id := strings.TrimSuffix(strings.TrimSuffix(name, ".prompt.md"), ".md")
				if seen[id] {
					continue
				}
				fullPath := filepath.Join(dirPath, name)
				if item, err := r.parsePromptFile(fullPath, sourceName, override); err == nil && item != nil {
					seen[id] = true
					results = append(results, *item)
				}
			}
		}
	}

	// 1. Project Local: {projectRoot}/.sdlc/prompts/
	if r.projectRoot != "" {
		addItemsFromDir(filepath.Join(r.projectRoot, ".sdlc", "prompts"), "PROJECT_LOCAL", true)
	}

	// 2. Project GitHub Prompts: {projectRoot}/.github/prompts/
	if r.projectRoot != "" {
		addItemsFromDir(filepath.Join(r.projectRoot, ".github", "prompts"), "GITHUB_PROMPTS", true)
	}

	// 3. Registered Custom Sources
	for _, src := range r.customSources {
		addItemsFromDir(src.Path, "CUSTOM_DIR", true)
	}

	// 4. User System: ~/.config/meta-orchestrator/prompts/
	if r.systemDir != "" {
		addItemsFromDir(r.systemDir, "USER_SYSTEM", false)
	}

	// 5. Default Built-in Templates (fallback if empty)
	if len(results) == 0 {
		results = []PromptItemDTO{
			{
				ID:             "generate-technical-document",
				Name:           "Generate Technical Document from PRD",
				Source:         "BUILTIN",
				Role:           "Staff Software Engineer",
				Description:    "Iterative deep-dive producing validated technical documents from PRD and architecture context",
				Variables:      []string{"epic", "prd_source", "rfc_url", "milestone"},
				SystemOverride: false,
				Compatible:     true,
			},
			{
				ID:             "prd-to-jira-user-story",
				Name:           "PRD to Jira User Story Parser",
				Source:         "BUILTIN",
				Role:           "Product Manager / QA Architect",
				Description:    "Parses PRD acceptance criteria into structured Jira user stories with MCP Atlassian bridge",
				Variables:      []string{"parent_issue_key", "user_story", "prd_url"},
				SystemOverride: false,
				Compatible:     true,
			},
			{
				ID:             "task-breakdown",
				Name:           "Task Breakdown & WBS Decomposition",
				Source:         "BUILTIN",
				Role:           "Lead Developer",
				Description:    "Breaks technical specifications into verifiable implementation tasks with write-lock boundaries",
				Variables:      []string{"epic", "user_story"},
				SystemOverride: false,
				Compatible:     true,
			},
		}
	}

	return results
}

var doubleVarRegex = regexp.MustCompile(`\{\{([a-zA-Z0-9_\-]+)\}\}`)
var singleVarRegex = regexp.MustCompile(`\{([a-zA-Z0-9_\-]+)\}`)
var tableParamRegex = regexp.MustCompile(`\|\s*` + "`?" + `([a-zA-Z0-9_\-]+)` + "`?" + `\s*\|\s*([^|]+)\s*\|`)
var headingRegex = regexp.MustCompile(`(?m)^#\s+(.+)$`)
var roleRegex = regexp.MustCompile(`(?i)\*\*Role:\*\*\s*([^\n|]+)`)

func (r *PromptRegistry) parsePromptFile(filePath string, sourceName string, override bool) (*PromptItemDTO, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	content := string(bytes)
	fileName := filepath.Base(filePath)
	id := strings.TrimSuffix(strings.TrimSuffix(fileName, ".prompt.md"), ".md")

	// Extract Title
	name := id
	if match := headingRegex.FindStringSubmatch(content); len(match) > 1 {
		name = strings.TrimSpace(match[1])
	}

	// Extract Role
	role := "AI Agent"
	if match := roleRegex.FindStringSubmatch(content); len(match) > 1 {
		role = strings.TrimSpace(match[1])
	}

	// Extract Description
	desc := ""
	lines := strings.Split(content, "\n")
	for _, l := range lines {
		trimmed := strings.TrimSpace(l)
		if strings.HasPrefix(trimmed, "Purpose:") || strings.HasPrefix(trimmed, "**Purpose:**") {
			desc = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(trimmed, "**Purpose:**"), "Purpose:"))
			break
		} else if strings.HasPrefix(trimmed, "> ") && len(trimmed) > 10 && desc == "" {
			desc = strings.TrimPrefix(trimmed, "> ")
		}
	}
	if desc == "" {
		for _, l := range lines {
			trimmed := strings.TrimSpace(l)
			if len(trimmed) > 20 && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "-") && !strings.HasPrefix(trimmed, "|") {
				desc = trimmed
				if len(desc) > 160 {
					desc = desc[:157] + "..."
				}
				break
			}
		}
	}

	// Extract Variables
	varSet := make(map[string]bool)
	vars := make([]string, 0)


	addVar := func(v string) {
		v = strings.TrimSpace(v)
		// Ignore common markdown/noise words
		ignored := map[string]bool{
			"epic_technical_document": true,
			"page_id":                 true,
			"UNVERIFIED":              true,
		}
		if v != "" && !varSet[v] && !ignored[v] && len(v) < 32 {
			varSet[v] = true
			vars = append(vars, v)
		}
	}

	// 1. Double curly variables: {{feature_name}}
	for _, m := range doubleVarRegex.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			addVar(m[1])
		}
	}

	// 2. Single curly variables: {epic}, {prd_source}, {user_story}
	for _, m := range singleVarRegex.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			addVar(m[1])
		}
	}

	// 3. Table parameters
	for _, m := range tableParamRegex.FindAllStringSubmatch(content, -1) {
		if len(m) > 1 {
			paramName := strings.TrimSpace(m[1])
			if paramName != "Parameter" && paramName != "Field" && paramName != "Name" {
				addVar(paramName)
			}
		}
	}

	return &PromptItemDTO{
		ID:             id,
		Name:           name,
		Source:         sourceName,
		SourcePath:     filePath,
		Role:           role,
		Description:    desc,
		Variables:      vars,
		RawContent:     content,
		SystemOverride: override,
		Compatible:     true,
	}, nil
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
// 2. GitHub Prompts: {projectRoot}/.github/prompts/{templateName}
// 3. Custom Registered Sources
// 4. User System: ~/.config/meta-orchestrator/prompts/{templateName}
// 5. Built-in: prompts/{templateName}
func (r *PromptRegistry) ResolveTemplate(templateName string) (string, error) {
	candidates := []string{templateName}
	if !strings.HasSuffix(templateName, ".md") {
		candidates = append(candidates, templateName+".prompt.md", templateName+".md")
	}

	searchDirs := []string{}
	if r.projectRoot != "" {
		searchDirs = append(searchDirs,
			filepath.Join(r.projectRoot, ".sdlc", "prompts"),
			filepath.Join(r.projectRoot, ".github", "prompts"),
		)
	}

	r.mu.RLock()
	for _, src := range r.customSources {
		searchDirs = append(searchDirs, src.Path)
	}
	r.mu.RUnlock()

	if r.systemDir != "" {
		searchDirs = append(searchDirs, r.systemDir)
	}
	searchDirs = append(searchDirs, r.builtinsDir)

	for _, dir := range searchDirs {
		for _, c := range candidates {
			target := filepath.Join(dir, c)
			if data, err := os.ReadFile(target); err == nil {
				return string(data), nil
			}
		}
	}

	return "", fmt.Errorf("prompt template '%s' not found across local, github, custom, or system paths", templateName)
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

// RenderCustom substitutes arbitrary key-value pairs (both {{key}} and {key}).
func (r *PromptRegistry) RenderCustom(template string, params map[string]string) string {
	out := template
	for k, v := range params {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
		out = strings.ReplaceAll(out, "{"+k+"}", v)
		out = strings.ReplaceAll(out, "$"+k, v)
	}
	return out
}
