package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// ConnectorMCPProvider provides active MCP server configurations.
type ConnectorMCPProvider interface {
	GetMCPExportConfig() map[string]interface{}
}

// MultiSourceSkillResolver coordinates cascading skill discovery across 4 tiers + dynamic MCP connectors.
type MultiSourceSkillResolver struct {
	mu           sync.RWMutex
	projectDir   string
	userHomeDir  string
	builtins     map[string]*types.UniversalSkillContract
	cachedSkills map[string]*types.UniversalSkillContract
	mcpProvider  ConnectorMCPProvider

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
// 1. Dynamic MCP provider (live connectors manager)
// 2. Project Local .sdlc/mcp.json or .mcp.json
// 3. Project Local: {projectDir}/.sdlc/skills/{skillName}
// 4. User System: ~/.config/meta-orchestrator/skills/{skillName}
// 5. Built-in defaults
func (r *MultiSourceSkillResolver) ResolveSkill(skillName string) (*types.UniversalSkillContract, error) {
	r.mu.RLock()
	cached, ok := r.cachedSkills[skillName]
	r.mu.RUnlock()
	if ok {
		return cached, nil
	}

	// 1. Dynamic MCP provider
	r.mu.RLock()
	provider := r.mcpProvider
	r.mu.RUnlock()
	if provider != nil {
		if skill := r.resolveFromMCPProvider(skillName, provider); skill != nil {
			r.cache(skillName, skill)
			return skill, nil
		}
	}

	// 2. Project Local .sdlc/mcp.json or .mcp.json
	if r.projectDir != "" {
		for _, rel := range []string{".sdlc/mcp.json", ".mcp.json"} {
			mcpPath := filepath.Join(r.projectDir, rel)
			if tools, err := r.mcpAdapter.ParseConfigFile(mcpPath); err == nil {
				for _, t := range tools {
					if t.Name == skillName || "mcp_"+t.Name == skillName {
						r.cache(skillName, t)
						return t, nil
					}
				}
			}
		}
	}

	// 3. Project Local .sdlc/skills/
	if r.projectDir != "" {
		localPath := filepath.Join(r.projectDir, ".sdlc", "skills", skillName)
		if skill, err := r.inspectAndParseDirectory(localPath); err == nil && skill != nil {
			r.cache(skillName, skill)
			return skill, nil
		}
	}

	// 4. User System
	if r.userHomeDir != "" {
		systemPath := filepath.Join(r.userHomeDir, ".config", "meta-orchestrator", "skills", skillName)
		if skill, err := r.inspectAndParseDirectory(systemPath); err == nil && skill != nil {
			r.cache(skillName, skill)
			return skill, nil
		}
	}

	// 5. Built-ins
	r.mu.RLock()
	builtin, exists := r.builtins[skillName]
	r.mu.RUnlock()
	if exists {
		return builtin, nil
	}

	return nil, fmt.Errorf("skill '%s' not found across local, system, or built-in registries", skillName)
}

func (r *MultiSourceSkillResolver) resolveFromMCPProvider(skillName string, provider ConnectorMCPProvider) *types.UniversalSkillContract {
	cfg := provider.GetMCPExportConfig()
	servers, ok := cfg["mcpServers"].(map[string]interface{})
	if !ok {
		return nil
	}

	for serverName, sVal := range servers {
		if serverName == skillName || "mcp_"+serverName == skillName {
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
				return r.mcpAdapter.ConvertServerConfig(serverName, MCPServerConfig{
					Command:     cmd,
					Args:        args,
					Environment: envMap,
				}, "connectors_manager")
			}
		}
	}
	return nil
}

// ListSkills returns all available skills across built-ins, dynamic MCP connectors, project local, and system registries.
func (r *MultiSourceSkillResolver) ListSkills() ([]*types.UniversalSkillContract, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*types.UniversalSkillContract
	seen := make(map[string]bool)

	// 1. Built-in tools
	for _, b := range r.builtins {
		if !seen[b.Name] {
			result = append(result, b)
			seen[b.Name] = true
		}
	}

	// 2. Dynamic MCP provider (live connectors manager)
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
					if !seen[toolContract.Name] {
						result = append(result, toolContract)
						seen[toolContract.Name] = true
					}
				}
			}
		}
	}

	// 3. Project Local .sdlc/mcp.json or .mcp.json
	if r.projectDir != "" {
		for _, rel := range []string{".sdlc/mcp.json", ".mcp.json"} {
			mcpPath := filepath.Join(r.projectDir, rel)
			if tools, err := r.mcpAdapter.ParseConfigFile(mcpPath); err == nil {
				for _, t := range tools {
					if !seen[t.Name] {
						result = append(result, t)
						seen[t.Name] = true
					}
				}
			}
		}
	}

	// 4. Project Local .sdlc/skills/
	if r.projectDir != "" {
		localSkillsDir := filepath.Join(r.projectDir, ".sdlc", "skills")
		if entries, err := os.ReadDir(localSkillsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() && !seen[e.Name()] {
					if sk, err := r.inspectAndParseDirectory(filepath.Join(localSkillsDir, e.Name())); err == nil && sk != nil {
						result = append(result, sk)
						seen[sk.Name] = true
					}
				}
			}
		}
	}

	// 5. User System skills
	if r.userHomeDir != "" {
		systemSkillsDir := filepath.Join(r.userHomeDir, ".config", "meta-orchestrator", "skills")
		if entries, err := os.ReadDir(systemSkillsDir); err == nil {
			for _, e := range entries {
				if e.IsDir() && !seen[e.Name()] {
					if sk, err := r.inspectAndParseDirectory(filepath.Join(systemSkillsDir, e.Name())); err == nil && sk != nil {
						result = append(result, sk)
						seen[sk.Name] = true
					}
				}
			}
		}
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
