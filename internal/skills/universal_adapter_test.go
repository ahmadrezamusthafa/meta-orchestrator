package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestClaudeSkillAdapter(t *testing.T) {
	adapter := NewClaudeSkillAdapter()
	tmpDir := t.TempDir()

	content := `---
name: web_scraper_claude
description: Scrapes HTML and extracts structured markdown
required_roles:
  - lead_developer
  - atdd_qa_engineer
timeout_seconds: 60
requires_network: true
---
# Web Scraper Skill
This skill extracts text and tables from URLs.
`
	if err := os.WriteFile(filepath.Join(tmpDir, "SKILL.md"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	contract, err := adapter.ParseDirectory(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse Claude skill: %v", err)
	}

	if contract.Name != "web_scraper_claude" {
		t.Errorf("expected name 'web_scraper_claude', got '%s'", contract.Name)
	}
	if contract.SourceFormat != types.SkillFormatClaude {
		t.Errorf("expected format Claude, got '%s'", contract.SourceFormat)
	}
	if contract.TimeoutSeconds != 60 {
		t.Errorf("expected timeout 60, got %d", contract.TimeoutSeconds)
	}
	if !contract.RequiresNetwork {
		t.Errorf("expected requires_network to be true")
	}
	if len(contract.RequiredRoles) != 2 {
		t.Errorf("expected 2 required roles, got %d", len(contract.RequiredRoles))
	}
}

func TestBMADSkillAdapter(t *testing.T) {
	adapter := NewBMADSkillAdapter()
	tmpDir := t.TempDir()

	yamlContent := `
id: atdd-generator-bmad
name: ATDD Suite Generator
description: Ingests PRD and codebase AST to generate executable Cucumber/Playwright test suites
methodology: bmad
allowed_roles:
  - atdd_qa_engineer
handoff_to: lead_developer
isolation: subprocess
timeout_seconds: 180
parameters:
  prd_path:
    type: string
  ast_slice:
    type: object
`
	if err := os.WriteFile(filepath.Join(tmpDir, "bmad-skill.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write bmad-skill.yaml: %v", err)
	}

	contract, err := adapter.ParseDirectory(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse BMAD skill: %v", err)
	}

	if contract.Name != "ATDD Suite Generator" {
		t.Errorf("expected name 'ATDD Suite Generator', got '%s'", contract.Name)
	}
	if contract.SourceFormat != types.SkillFormatBMAD {
		t.Errorf("expected format BMAD, got '%s'", contract.SourceFormat)
	}
	if len(contract.RequiredRoles) != 1 || contract.RequiredRoles[0] != "atdd_qa_engineer" {
		t.Errorf("unexpected required roles: %v", contract.RequiredRoles)
	}
}

func TestSuperpowerSkillAdapter(t *testing.T) {
	adapter := NewSuperpowerSkillAdapter()
	tmpDir := t.TempDir()

	yamlContent := `
name: git_worktree_manager
description: Manage atomic isolated git worktrees
command: git
allow_host_access: true
timeout_seconds: 90
`
	if err := os.WriteFile(filepath.Join(tmpDir, "superpower.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write superpower.yaml: %v", err)
	}

	contract, err := adapter.ParseDirectory(tmpDir)
	if err != nil {
		t.Fatalf("failed to parse Superpower skill: %v", err)
	}

	if contract.Name != "git_worktree_manager" {
		t.Errorf("expected 'git_worktree_manager', got '%s'", contract.Name)
	}
	if contract.SourceFormat != types.SkillFormatSuperpower {
		t.Errorf("expected format Superpower, got '%s'", contract.SourceFormat)
	}
	if contract.Isolation != types.IsolationHost {
		t.Errorf("expected IsolationHost, got '%s'", contract.Isolation)
	}
}

func TestMCPSkillAdapter(t *testing.T) {
	adapter := NewMCPSkillAdapter()
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "mcp.json")

	jsonContent := `{
  "tools": [
    {
      "name": "read_postgres_schema",
      "description": "Introspect database schema and constraints",
      "inputSchema": {
        "type": "object",
        "properties": {
          "table": { "type": "string" }
        }
      }
    }
  ]
}`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write mcp.json: %v", err)
	}

	tools, err := adapter.ParseConfigFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to parse MCP config: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 MCP tool, got %d", len(tools))
	}
	if tools[0].Name != "read_postgres_schema" {
		t.Errorf("expected name 'read_postgres_schema', got '%s'", tools[0].Name)
	}
	if tools[0].SourceFormat != types.SkillFormatMCP {
		t.Errorf("expected format MCP, got '%s'", tools[0].SourceFormat)
	}
}

func TestOpenAISkillAdapter(t *testing.T) {
	adapter := NewOpenAISkillAdapter()
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "openai_tools.json")

	jsonContent := `[
  {
    "type": "function",
    "function": {
      "name": "calculate_hash",
      "description": "Calculates SHA-256 hash of a string",
      "parameters": {
        "type": "object",
        "properties": {
          "data": { "type": "string" }
        }
      }
    }
  }
]`
	if err := os.WriteFile(jsonPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write openai_tools.json: %v", err)
	}

	tools, err := adapter.ParseJSONFile(jsonPath)
	if err != nil {
		t.Fatalf("failed to parse OpenAI tools: %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Name != "calculate_hash" {
		t.Errorf("expected 'calculate_hash', got '%s'", tools[0].Name)
	}
	if tools[0].SourceFormat != types.SkillFormatOpenAI {
		t.Errorf("expected format OpenAI, got '%s'", tools[0].SourceFormat)
	}
}

func TestMultiSourceResolverAndRBAC(t *testing.T) {
	tmpProject := t.TempDir()
	resolver := NewMultiSourceSkillResolver(tmpProject)

	// Built-in resolution
	builtin, err := resolver.ResolveSkill("resolve_symlinks")
	if err != nil || builtin == nil {
		t.Fatalf("failed to resolve built-in skill: %v", err)
	}

	// Project Local override
	localSkillDir := filepath.Join(tmpProject, ".sdlc", "skills", "custom_local_tool")
	if err := os.MkdirAll(localSkillDir, 0755); err != nil {
		t.Fatalf("failed to mkdir: %v", err)
	}
	skillContent := `---
name: custom_local_tool
description: Project specific tool
required_roles:
  - devops_superpower
---
Project local instructions
`
	if err := os.WriteFile(filepath.Join(localSkillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatalf("failed to write local SKILL.md: %v", err)
	}

	localSkill, err := resolver.ResolveSkill("custom_local_tool")
	if err != nil || localSkill == nil {
		t.Fatalf("failed to resolve local skill: %v", err)
	}
	if localSkill.Name != "custom_local_tool" {
		t.Errorf("expected 'custom_local_tool', got '%s'", localSkill.Name)
	}

	// Test RBAC rejection
	executor := NewStandardSkillExecutor()
	ctx := context.Background()

	unauthorizedReq := &types.SkillExecutionRequest{
		TaskID:     "task-rbac-1",
		CallerRole: "vue_frontend_engineer", // Not allowed
	}
	_, err = executor.Execute(ctx, localSkill, unauthorizedReq, nil)
	if err == nil {
		t.Fatalf("expected RBAC rejection for vue_frontend_engineer on devops tool")
	}

	authorizedReq := &types.SkillExecutionRequest{
		TaskID:     "task-rbac-1",
		CallerRole: "devops_superpower", // Allowed
	}
	res, err := executor.Execute(ctx, localSkill, authorizedReq, nil)
	if err != nil {
		t.Fatalf("expected authorized execution to succeed: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
}

type mockMCPProvider struct {
	cfg map[string]interface{}
}

func (m *mockMCPProvider) GetMCPExportConfig() map[string]interface{} {
	return m.cfg
}

func TestMultiSourceResolver_DynamicMCPIntegration(t *testing.T) {
	tmpProject := t.TempDir()
	resolver := NewMultiSourceSkillResolver(tmpProject)

	mockProv := &mockMCPProvider{
		cfg: map[string]interface{}{
			"mcpServers": map[string]interface{}{
				"jira": map[string]interface{}{
					"command": "echo",
					"args":    []string{"jira-mcp-active"},
					"env": map[string]interface{}{
						"JIRA_URL": "https://test.atlassian.net",
					},
				},
				"github": map[string]interface{}{
					"command": "echo",
					"args":    []string{"github-mcp-active"},
				},
			},
		},
	}

	resolver.SetMCPProvider(mockProv)

	// 1. Resolve dynamic MCP skill
	skill, err := resolver.ResolveSkill("jira")
	if err != nil || skill == nil {
		t.Fatalf("failed to dynamically resolve 'jira' MCP skill: %v", err)
	}
	if skill.SourceFormat != types.SkillFormatMCP {
		t.Errorf("expected SourceFormat MCP, got '%s'", skill.SourceFormat)
	}
	if skill.Command != "echo" {
		t.Errorf("expected command 'echo', got '%s'", skill.Command)
	}

	// 2. List all skills (must include built-ins and active MCP connectors)
	allSkills, err := resolver.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	hasJira := false
	hasGitHub := false
	hasBuiltin := false
	for _, s := range allSkills {
		if s.Name == "jira" {
			hasJira = true
		}
		if s.Name == "github" {
			hasGitHub = true
		}
		if s.Name == "resolve_symlinks" {
			hasBuiltin = true
		}
	}
	if !hasJira || !hasGitHub || !hasBuiltin {
		t.Errorf("ListSkills missing items: jira=%v, github=%v, builtin=%v", hasJira, hasGitHub, hasBuiltin)
	}

	// 3. Execute MCP skill through StandardSkillExecutor
	executor := NewStandardSkillExecutor()
	execReq := &types.SkillExecutionRequest{
		TaskID: "task-mcp-1",
	}
	res, err := executor.Execute(context.Background(), skill, execReq, nil)
	if err != nil {
		t.Fatalf("failed to execute MCP skill: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if !strings.Contains(res.Stdout, "jira-mcp-active") {
		t.Errorf("expected output to contain 'jira-mcp-active', got '%s'", res.Stdout)
	}
}

func TestSkillCompatibilityAudit(t *testing.T) {
	// 1. Fully compatible native skill
	goodSkill := &types.UniversalSkillContract{
		Name:            "safe_code_analyzer",
		Description:     "Parses code syntax trees and returns AST metrics",
		SourceFormat:    types.SkillFormatNative,
		Isolation:       types.IsolationSubprocess,
		TimeoutSeconds:  60,
		RequiresNetwork: false,
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"file": map[string]interface{}{"type": "string"},
			},
		},
	}
	compat := CheckSkillCompatibility(goodSkill)
	if compat.Status != types.CompatibilityStatusCompatible {
		t.Errorf("expected compatible status, got '%s'", compat.Status)
	}
	if compat.Score < 80 {
		t.Errorf("expected score >= 80, got %d", compat.Score)
	}
	if !compat.RuntimeReady || !compat.SandboxSafe {
		t.Errorf("expected RuntimeReady and SandboxSafe to be true")
	}

	// 2. Warning skill: host isolation and missing description details
	warningSkill := &types.UniversalSkillContract{
		Name:            "host_script_runner",
		Description:     "short",
		SourceFormat:    types.SkillFormatSuperpower,
		Isolation:       types.IsolationHost,
		TimeoutSeconds:  800, // > 600s
		RequiresNetwork: true,
	}
	compatWarn := CheckSkillCompatibility(warningSkill)
	if compatWarn.Status != types.CompatibilityStatusWarning {
		t.Errorf("expected warning status, got '%s'", compatWarn.Status)
	}
	if compatWarn.SandboxSafe {
		t.Errorf("expected SandboxSafe to be false for host isolation")
	}

	// 3. Incompatible skill: invalid name, missing required files
	incompatibleSkill := &types.UniversalSkillContract{
		Name:           "invalid name with spaces!@#",
		SourceFormat:   types.SkillFormatClaude,
		SourceLocation: "/non/existent/path/for/sure",
	}
	compatBad := CheckSkillCompatibility(incompatibleSkill)
	if compatBad.Status != types.CompatibilityStatusIncompatible {
		t.Errorf("expected incompatible status, got '%s'", compatBad.Status)
	}
	if compatBad.Score != 0 && compatBad.Score > 50 {
		t.Errorf("expected low score for incompatible skill, got %d", compatBad.Score)
	}
	if compatBad.RuntimeReady {
		t.Errorf("expected RuntimeReady to be false")
	}
}

func TestClaudeSkillAutoDiscoveryAndExecution(t *testing.T) {
	tmpProject := t.TempDir()

	// Create .claude/skills/code_generator/SKILL.md with a runnable script
	claudeSkillsDir := filepath.Join(tmpProject, ".claude", "skills", "code_generator")
	if err := os.MkdirAll(filepath.Join(claudeSkillsDir, "scripts"), 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	skillMD := `---
name: code_generator
description: Generates clean boilerplate files for TypeScript microservices
timeout_seconds: 45
requires_network: false
---
# Instructions
Always format output in valid TypeScript.
`
	if err := os.WriteFile(filepath.Join(claudeSkillsDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	// Write mock script run.sh
	scriptContent := "#!/bin/sh\necho 'GENERATED_BOILERPLATE_SUCCESS'\n"
	scriptPath := filepath.Join(claudeSkillsDir, "scripts", "run.sh")
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write run.sh: %v", err)
	}

	resolver := NewMultiSourceSkillResolver(tmpProject)

	// Verify auto-discovery from .claude/skills/
	skill, err := resolver.ResolveSkill("code_generator")
	if err != nil {
		t.Fatalf("failed to auto-resolve Claude skill: %v", err)
	}
	if skill.SourceFormat != types.SkillFormatClaude {
		t.Errorf("expected source format Claude, got '%s'", skill.SourceFormat)
	}
	if skill.Command != "bash" {
		t.Errorf("expected command 'bash', got '%s'", skill.Command)
	}
	if skill.Compatibility == nil || skill.Compatibility.Status == types.CompatibilityStatusIncompatible {
		t.Errorf("expected valid compatibility report, got: %+v", skill.Compatibility)
	}

	// Execute skill
	executor := NewStandardSkillExecutor()
	res, err := executor.Execute(context.Background(), skill, &types.SkillExecutionRequest{TaskID: "task-cg-1"}, nil)
	if err != nil {
		t.Fatalf("execution error: %v", err)
	}
	if !strings.Contains(res.Stdout, "GENERATED_BOILERPLATE_SUCCESS") {
		t.Errorf("expected script output, got '%s'", res.Stdout)
	}
}

func TestModularEnableDisableAndPersistence(t *testing.T) {
	tmpProject := t.TempDir()
	resolver := NewMultiSourceSkillResolver(tmpProject)

	// Builtin skill is enabled by default
	skill, err := resolver.ResolveSkill("resolve_symlinks")
	if err != nil || skill == nil {
		t.Fatalf("expected resolve_symlinks to resolve: %v", err)
	}
	if !skill.Enabled {
		t.Errorf("expected skill to be enabled by default")
	}

	// Disable skill
	if err := resolver.ToggleSkill("resolve_symlinks", false); err != nil {
		t.Fatalf("failed to toggle skill off: %v", err)
	}

	// Resolution should now fail
	_, err = resolver.ResolveSkill("resolve_symlinks")
	if err == nil {
		t.Errorf("expected resolution to fail for disabled skill")
	}

	// ListSkills should not contain resolve_symlinks
	activeSkills, err := resolver.ListSkills()
	if err != nil {
		t.Fatalf("ListSkills failed: %v", err)
	}
	for _, s := range activeSkills {
		if s.Name == "resolve_symlinks" {
			t.Errorf("disabled skill should not be present in active ListSkills()")
		}
	}

	// ListAllSkillsWithDetails should contain it with Enabled=false
	allDetails, err := resolver.ListAllSkillsWithDetails(false)
	if err != nil {
		t.Fatalf("ListAllSkillsWithDetails failed: %v", err)
	}
	found := false
	for _, s := range allDetails {
		if s.Name == "resolve_symlinks" {
			found = true
			if s.Enabled {
				t.Errorf("expected Enabled=false in allDetails")
			}
		}
	}
	if !found {
		t.Errorf("expected resolve_symlinks in allDetails")
	}

	// Verify persistence: recreate resolver from the same directory
	resolver2 := NewMultiSourceSkillResolver(tmpProject)
	if resolver2.IsSkillEnabled("resolve_symlinks") {
		t.Errorf("expected persisted config to keep resolve_symlinks disabled")
	}

	// Re-enable
	if err := resolver2.ToggleSkill("resolve_symlinks", true); err != nil {
		t.Fatalf("failed to re-enable skill: %v", err)
	}
	skillRestored, err := resolver2.ResolveSkill("resolve_symlinks")
	if err != nil || skillRestored == nil {
		t.Fatalf("expected resolve_symlinks to resolve after re-enabling: %v", err)
	}
}

func TestCustomSourceRegistrationAndAudit(t *testing.T) {
	tmpProject := t.TempDir()
	customSourceDir := t.TempDir()

	// Create custom skill inside customSourceDir
	customSkillDir := filepath.Join(customSourceDir, "custom_audit_tool")
	if err := os.MkdirAll(customSkillDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	skillMD := `---
name: custom_audit_tool
description: Inspects pull request diffs for OWASP Top 10 vulnerabilities
---
# Security Auditor Instructions
Check for SQL injection and broken access controls.
`
	if err := os.WriteFile(filepath.Join(customSkillDir, "SKILL.md"), []byte(skillMD), 0644); err != nil {
		t.Fatalf("failed to write SKILL.md: %v", err)
	}

	resolver := NewMultiSourceSkillResolver(tmpProject)

	// Pre-flight compatibility audit before registration
	preFlight, err := resolver.CheckPathCompatibility(customSourceDir)
	if err != nil {
		t.Fatalf("pre-flight check failed: %v", err)
	}
	if len(preFlight) != 1 {
		t.Fatalf("expected 1 discovered skill in pre-flight, got %d", len(preFlight))
	}
	if preFlight[0].Name != "custom_audit_tool" {
		t.Errorf("expected 'custom_audit_tool', got '%s'", preFlight[0].Name)
	}
	if preFlight[0].Compatibility.Status == types.CompatibilityStatusIncompatible {
		t.Errorf("expected compatible or warning status, got incompatible")
	}

	// Register source
	src, discovered, err := resolver.RegisterSource("Security Skills", customSourceDir, types.SkillFormatClaude)
	if err != nil {
		t.Fatalf("RegisterSource failed: %v", err)
	}
	if src.Name != "Security Skills" || src.SkillCount != 1 {
		t.Errorf("unexpected registered source: %+v", src)
	}
	if len(discovered) != 1 {
		t.Errorf("expected 1 discovered skill: %d", len(discovered))
	}

	// Verify resolution
	skill, err := resolver.ResolveSkill("custom_audit_tool")
	if err != nil || skill == nil {
		t.Fatalf("failed to resolve custom registered skill: %v", err)
	}
	if skill.SourceType != "custom_dir" {
		t.Errorf("expected SourceType 'custom_dir', got '%s'", skill.SourceType)
	}

	// Unregister source
	if err := resolver.UnregisterSource(src.ID); err != nil {
		t.Fatalf("UnregisterSource failed: %v", err)
	}
	if len(resolver.ListSources()) != 0 {
		t.Errorf("expected 0 sources after unregister, got %d", len(resolver.ListSources()))
	}
}


