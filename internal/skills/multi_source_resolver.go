package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ConnectorMCPProvider provides active MCP server configurations.
type ConnectorMCPProvider interface {
	GetMCPExportConfig() map[string]interface{}
}

// MultiSourceSkillResolver coordinates cascading skill discovery across built-ins, dynamic MCP,
// auto-detected Claude directories, project local skills, and user-registered custom sources.
type MultiSourceSkillResolver struct {
	mu           sync.RWMutex
	projectDir   string
	userHomeDir  string
	builtins     map[string]*types.UniversalSkillContract
	cachedSkills map[string]*types.UniversalSkillContract
	mcpProvider  ConnectorMCPProvider
	configMgr    *ConfigManager

	claudeAdapter     *ClaudeSkillAdapter
	bmadAdapter       *BMADSkillAdapter
	superpowerAdapter *SuperpowerSkillAdapter
	mcpAdapter        *MCPSkillAdapter
	openaiAdapter     *OpenAISkillAdapter
}

// NewMultiSourceSkillResolver initializes the cascading resolver with persistence and adapters.
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
		configMgr:         NewConfigManager(projectDir, userHome),
	}

	r.configMgr.SetOnUpdateCallback(func() {
		r.mu.Lock()
		r.cachedSkills = make(map[string]*types.UniversalSkillContract)
		r.mu.Unlock()
	})

	r.registerDefaultBuiltins()
	return r
}

// SetMCPProvider attaches a dynamic MCP server provider (e.g. connectors.Manager).
func (r *MultiSourceSkillResolver) SetMCPProvider(provider ConnectorMCPProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mcpProvider = provider
	r.cachedSkills = make(map[string]*types.UniversalSkillContract)
}

func (r *MultiSourceSkillResolver) registerDefaultBuiltins() {
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "resolve_symlinks",
		Description:     "Resolve symlinked repo paths and directory structures safely",
		SourceFormat:    types.SkillFormatNative,
		SourceType:      "builtin",
		Enabled:         true,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  30,
		RequiresNetwork: false,
		RequiredRoles:   []string{"system_architect", "lead_developer", "devops_superpower"},
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "docker_compose_up",
		Description:     "Spin up isolated multi-container workspace services",
		SourceFormat:    types.SkillFormatNative,
		SourceType:      "builtin",
		Enabled:         true,
		Isolation:       types.IsolationHost,
		TimeoutSeconds:  180,
		RequiresNetwork: false,
		RequiredRoles:   []string{"devops_superpower"},
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "run_playwright_e2e",
		Description:     "Run automated Playwright browser tests with video recording",
		SourceFormat:    types.SkillFormatNative,
		SourceType:      "builtin",
		Enabled:         true,
		Isolation:       types.IsolationDocker,
		TimeoutSeconds:  300,
		RequiresNetwork: true,
		RequiredRoles:   []string{"atdd_qa_engineer", "lead_developer"},
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "read_ast_node",
		Description:     "Extract precise AST syntactic nodes via Tree-sitter",
		SourceFormat:    types.SkillFormatNative,
		SourceType:      "builtin",
		Enabled:         true,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  15,
		RequiresNetwork: false,
		RequiredRoles:   []string{"product_manager", "system_architect", "atdd_qa_engineer", "lead_developer"},
	})
	r.RegisterBuiltin(&types.UniversalSkillContract{
		Name:            "git_atomic_commit",
		Description:     "Execute signed atomic git commit with cryptographic SHA hash",
		SourceFormat:    types.SkillFormatNative,
		SourceType:      "builtin",
		Enabled:         true,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  30,
		RequiresNetwork: false,
		RequiredRoles:   []string{"lead_developer", "devops_superpower"},
	})
}

// RegisterBuiltin registers a built-in default skill.
func (r *MultiSourceSkillResolver) RegisterBuiltin(skill *types.UniversalSkillContract) {
	r.mu.Lock()
	defer r.mu.Unlock()
	skill.Enabled = r.configMgr.IsSkillEnabled(skill.Name)
	skill.Compatibility = CheckSkillCompatibility(skill)
	r.builtins[skill.Name] = skill
}

// GetConfigManager returns the internal configuration manager.
func (r *MultiSourceSkillResolver) GetConfigManager() *ConfigManager {
	return r.configMgr
}

// ToggleSkill changes a skill's enabled status and invalidates resolution caches.
func (r *MultiSourceSkillResolver) ToggleSkill(skillName string, enabled bool) error {
	r.mu.Lock()
	delete(r.cachedSkills, skillName)
	r.mu.Unlock()
	return r.configMgr.SetSkillEnabled(skillName, enabled)
}

// IsSkillEnabled returns true if the skill is enabled.
func (r *MultiSourceSkillResolver) IsSkillEnabled(skillName string) bool {
	return r.configMgr.IsSkillEnabled(skillName)
}

// ListSources returns all registered custom skill source directories.
func (r *MultiSourceSkillResolver) ListSources() []SkillSource {
	return r.configMgr.ListSources()
}

// RegisterSource audits and saves a custom directory as a persistent skill source.
func (r *MultiSourceSkillResolver) RegisterSource(name, path string, format types.SkillFormat) (*SkillSource, []*types.UniversalSkillContract, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, nil, fmt.Errorf("source path inaccessible: %w", err)
	}
	if !stat.IsDir() {
		return nil, nil, fmt.Errorf("source path is not a directory: %s", path)
	}

	discovered, err := AuditDirectory(path, r.claudeAdapter, r.mcpAdapter)
	if err != nil {
		return nil, nil, fmt.Errorf("compatibility audit failed on %s: %w", path, err)
	}

	if format == "" {
		format = types.SkillFormatClaude
		if len(discovered) > 0 && discovered[0].SourceFormat != "" {
			format = discovered[0].SourceFormat
		}
	}

	sourceID := fmt.Sprintf("src-%d", time.Now().UnixNano())
	if name == "" {
		name = filepath.Base(path)
	}

	src := SkillSource{
		ID:         sourceID,
		Name:       name,
		Path:       path,
		Format:     format,
		Enabled:    true,
		SkillCount: len(discovered),
		AddedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if err := r.configMgr.AddSource(src); err != nil {
		return nil, nil, err
	}

	r.mu.Lock()
	r.cachedSkills = make(map[string]*types.UniversalSkillContract)
	r.mu.Unlock()

	return &src, discovered, nil
}

// UnregisterSource removes a custom source by ID.
func (r *MultiSourceSkillResolver) UnregisterSource(sourceID string) error {
	r.mu.Lock()
	r.cachedSkills = make(map[string]*types.UniversalSkillContract)
	r.mu.Unlock()
	return r.configMgr.RemoveSource(sourceID)
}

// CheckPathCompatibility audits an arbitrary path or directory without registering it.
func (r *MultiSourceSkillResolver) CheckPathCompatibility(targetPath string) ([]*types.UniversalSkillContract, error) {
	return AuditDirectory(targetPath, r.claudeAdapter, r.mcpAdapter)
}

// Rescan clears all in-memory caches and re-reads all directories.
func (r *MultiSourceSkillResolver) Rescan() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cachedSkills = make(map[string]*types.UniversalSkillContract)
}

// ResolveSkill resolves a single skill contract by name. Returns error if disabled or not found.
func (r *MultiSourceSkillResolver) ResolveSkill(skillName string) (*types.UniversalSkillContract, error) {
	if !r.configMgr.IsSkillEnabled(skillName) {
		return nil, fmt.Errorf("skill '%s' is disabled in skills configuration", skillName)
	}

	r.mu.RLock()
	cached, ok := r.cachedSkills[skillName]
	r.mu.RUnlock()
	if ok {
		return cached, nil
	}

	allSkills, err := r.ListSkills()
	if err != nil {
		return nil, err
	}

	for _, s := range allSkills {
		if s.Name == skillName || "mcp_"+s.Name == skillName {
			r.cache(skillName, s)
			return s, nil
		}
	}

	return nil, fmt.Errorf("skill '%s' not found across local, custom, system, or built-in registries", skillName)
}

// ListSkills returns all currently ENABLED skills for the AI worker loop.
func (r *MultiSourceSkillResolver) ListSkills() ([]*types.UniversalSkillContract, error) {
	return r.ListAllSkillsWithDetails(true)
}

// ListAllSkillsWithDetails returns all discovered skills with modular metadata and compatibility reports.
func (r *MultiSourceSkillResolver) ListAllSkillsWithDetails(enabledOnly bool) ([]*types.UniversalSkillContract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*types.UniversalSkillContract
	seen := make(map[string]bool)

	addSkill := func(s *types.UniversalSkillContract, defaultSourceType string) {
		if s == nil || s.Name == "" || seen[s.Name] {
			return
		}
		s.Enabled = r.configMgr.IsSkillEnabled(s.Name)
		if defaultSourceType != "" {
			s.SourceType = defaultSourceType
		}
		if s.Compatibility == nil {
			s.Compatibility = CheckSkillCompatibility(s)
		}
		if enabledOnly && !s.Enabled {
			return
		}
		seen[s.Name] = true
		result = append(result, s)
	}

	// Tier 1: Dynamic MCP provider (live connectors manager: JIRA, Confluence, etc.)
	if r.mcpProvider != nil {
		cfg := r.mcpProvider.GetMCPExportConfig()
		if servers, ok := cfg["mcpServers"].(map[string]interface{}); ok {
			for serverName, sVal := range servers {
				if confMap, ok := sVal.(map[string]interface{}); ok {
					cmd, _ := confMap["command"].(string)
					var args []string
					if rawArgs, ok := confMap["args"].([]string); ok {
						args = rawArgs
					} else if rawSlice, ok := confMap["args"].([]interface{}); ok {
						for _, a := range rawSlice {
							args = append(args, fmt.Sprint(a))
						}
					}
					envMap := make(map[string]string)
					if rawEnv, ok := confMap["env"].(map[string]string); ok {
						envMap = rawEnv
					} else if rawEnvMap, ok := confMap["env"].(map[string]interface{}); ok {
						for k, v := range rawEnvMap {
							envMap[k] = fmt.Sprint(v)
						}
					}
					toolContract := r.mcpAdapter.ConvertServerConfig(serverName, MCPServerConfig{
						Command:     cmd,
						Args:        args,
						Environment: envMap,
					}, "connectors_manager")
					addSkill(toolContract, "mcp_connector")
				}
			}
		}
	}

	// Tier 2: Project Local .sdlc/mcp.json or .mcp.json
	if r.projectDir != "" {
		for _, rel := range []string{".sdlc/mcp.json", ".mcp.json"} {
			mcpPath := filepath.Join(r.projectDir, rel)
			if tools, err := r.mcpAdapter.ParseConfigFile(mcpPath); err == nil {
				for _, t := range tools {
					addSkill(t, "project_local")
				}
			}
		}
	}

	// Tier 3: Project Local .sdlc/skills/
	if r.projectDir != "" {
		localSkillsDir := filepath.Join(r.projectDir, ".sdlc", "skills")
		if entries, err := os.ReadDir(localSkillsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					if sk, err := r.inspectAndParseDirectory(filepath.Join(localSkillsDir, e.Name())); err == nil && sk != nil {
						addSkill(sk, "project_local")
					}
				}
			}
		}
	}

	// Tier 4: Auto-detected Claude / Agent skill directories
	claudeCandidateDirs := []string{}
	if r.projectDir != "" {
		claudeCandidateDirs = append(claudeCandidateDirs,
			filepath.Join(r.projectDir, ".claude", "skills"),
			filepath.Join(r.projectDir, ".anthropic", "skills"),
			filepath.Join(r.projectDir, ".agents", "skills"),
		)
	}
	if r.userHomeDir != "" {
		claudeCandidateDirs = append(claudeCandidateDirs,
			filepath.Join(r.userHomeDir, ".claude", "skills"),
		)
	}

	for _, cDir := range claudeCandidateDirs {
		if entries, err := os.ReadDir(cDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					subPath := filepath.Join(cDir, e.Name())
					if sk, err := r.claudeAdapter.ParseDirectory(subPath); err == nil && sk != nil {
						addSkill(sk, "claude")
					}
				}
			}
		}
	}

	// Tier 5: Registered Custom Sources from configuration
	for _, src := range r.configMgr.ListSources() {
		if !src.Enabled {
			continue
		}
		if skills, err := AuditDirectory(src.Path, r.claudeAdapter, r.mcpAdapter); err == nil {
			for _, s := range skills {
				addSkill(s, "custom_dir")
			}
		}
	}

	// Tier 6: User System skills directory (~/.config/meta-orchestrator/skills/)
	if r.userHomeDir != "" {
		systemSkillsDir := filepath.Join(r.userHomeDir, ".config", "meta-orchestrator", "skills")
		if entries, err := os.ReadDir(systemSkillsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					if sk, err := r.inspectAndParseDirectory(filepath.Join(systemSkillsDir, e.Name())); err == nil && sk != nil {
						addSkill(sk, "system")
					}
				}
			}
		}
	}

	// Tier 7: Built-in tools
	for _, b := range r.builtins {
		addSkill(b, "builtin")
	}

	return result, nil
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
