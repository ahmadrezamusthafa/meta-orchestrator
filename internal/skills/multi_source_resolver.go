package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// MultiSourceSkillResolver coordinates cascading skill discovery across 4 tiers.
type MultiSourceSkillResolver struct {
	mu           sync.RWMutex
	projectDir   string
	userHomeDir  string
	builtins     map[string]*types.UniversalSkillContract
	cachedSkills map[string]*types.UniversalSkillContract

	claudeAdapter     *ClaudeSkillAdapter
	bmadAdapter       *BMADSkillAdapter
	superpowerAdapter *SuperpowerSkillAdapter
	mcpAdapter        *MCPSkillAdapter
	openaiAdapter     *OpenAISkillAdapter
}

// NewMultiSourceSkillResolver initializes the cascading resolver.
func NewMultiSourceSkillResolver(projectDir string) *MultiSourceSkillResolver {
	userHome, _ := os.UserHomeDir()
	r := &MultiSourceSkillResolver{
		projectDir:        projectDir,
		userHomeDir:       userHome,
		builtins:          make(map[string]*types.UniversalSkillContract),
		cachedSkills:      make(map[string]*types.UniversalSkillContract),
		claudeAdapter:     NewClaudeSkillAdapter(),
		bmadAdapter:       NewBMADSkillAdapter(),
		superpowerAdapter: NewSuperpowerSkillAdapter(),
		mcpAdapter:        NewMCPSkillAdapter(),
		openaiAdapter:     NewOpenAISkillAdapter(),
	}
	r.registerDefaultBuiltins()
	return r
}

func (r *MultiSourceSkillResolver) registerDefaultBuiltins() {
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "resolve_symlinks",
		Description:     "Resolve symlinked repo paths and directory structures safely",
		SourceFormat:    types.SkillFormatNative,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  30,
		RequiresNetwork: false,
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "docker_compose_up",
		Description:     "Spin up isolated multi-container workspace services",
		SourceFormat:    types.SkillFormatNative,
		Isolation:       types.IsolationHost,
		TimeoutSeconds:  180,
		RequiresNetwork: false,
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "run_playwright_e2e",
		Description:     "Run automated Playwright browser tests with video recording",
		SourceFormat:    types.SkillFormatNative,
		Isolation:       types.IsolationDocker,
		TimeoutSeconds:  300,
		RequiresNetwork: true,
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "read_ast_node",
		Description:     "Extract precise AST syntactic nodes via Tree-sitter",
		SourceFormat:    types.SkillFormatNative,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  15,
		RequiresNetwork: false,
	})
}

// RegisterBuiltin registers a built-in default skill.
func (r *MultiSourceSkillResolver) RegisterBuiltin(skill *types.UniversalSkillContract) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builtins[skill.Name] = skill
}

// ResolveSkill searches across cascading tiers:
// 1. Project Local: {projectDir}/.sdlc/skills/{skillName}
// 2. User System: ~/.config/meta-orchestrator/skills/{skillName}
// 3. Built-in defaults
func (r *MultiSourceSkillResolver) ResolveSkill(skillName string) (*types.UniversalSkillContract, error) {
	r.mu.RLock()
	cached, ok := r.cachedSkills[skillName]
	r.mu.RUnlock()
	if ok {
		return cached, nil
	}

	// 1. Project Local
	if r.projectDir != "" {
		localPath := filepath.Join(r.projectDir, ".sdlc", "skills", skillName)
		if skill, err := r.inspectAndParseDirectory(localPath); err == nil && skill != nil {
			r.cache(skillName, skill)
			return skill, nil
		}
	}

	// 2. User System
	if r.userHomeDir != "" {
		systemPath := filepath.Join(r.userHomeDir, ".config", "meta-orchestrator", "skills", skillName)
		if skill, err := r.inspectAndParseDirectory(systemPath); err == nil && skill != nil {
			r.cache(skillName, skill)
			return skill, nil
		}
	}

	// 3. Built-ins
	r.mu.RLock()
	builtin, exists := r.builtins[skillName]
	r.mu.RUnlock()
	if exists {
		return builtin, nil
	}

	return nil, fmt.Errorf("skill '%s' not found across local, system, or built-in registries", skillName)
}

func (r *MultiSourceSkillResolver) inspectAndParseDirectory(dirPath string) (*types.UniversalSkillContract, error) {
	stat, err := os.Stat(dirPath)
	if err != nil || !stat.IsDir() {
		return nil, fmt.Errorf("directory not found: %s", dirPath)
	}

	// Check Claude SKILL.md
	if _, err := os.Stat(filepath.Join(dirPath, "SKILL.md")); err == nil {
		return r.claudeAdapter.ParseDirectory(dirPath)
	}

	// Check BMAD manifest
	if _, err := os.Stat(filepath.Join(dirPath, "bmad-skill.yaml")); err == nil {
		return r.bmadAdapter.ParseDirectory(dirPath)
	}
	if _, err := os.Stat(filepath.Join(dirPath, "bmad-skill.json")); err == nil {
		return r.bmadAdapter.ParseDirectory(dirPath)
	}

	// Check Superpower manifest
	if _, err := os.Stat(filepath.Join(dirPath, "superpower.yaml")); err == nil {
		return r.superpowerAdapter.ParseDirectory(dirPath)
	}

	return nil, fmt.Errorf("unknown skill directory format in %s", dirPath)
}

func (r *MultiSourceSkillResolver) cache(name string, skill *types.UniversalSkillContract) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cachedSkills[name] = skill
}
