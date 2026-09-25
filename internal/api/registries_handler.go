package api

import (
	"net/http"
)

type RegistryDTO struct {
	Skills []SkillItemDTO `json:"skills"`
	Prompts []PromptItemDTO `json:"prompts"`
	Hooks   []HookItemDTO   `json:"hooks"`
}

type SkillItemDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"` // "PROJECT_LOCAL", "USER_SYSTEM", "REMOTE_GIT", "BUILTIN"
	Format      string `json:"format"` // "BMAD", "CLAUDE", "SUPERPOWER", "MCP", "OPENAI"
	Description string `json:"description"`
	RepoURL     string `json:"repo_url,omitempty"`
}

type PromptItemDTO struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Source         string   `json:"source"`
	Variables      []string `json:"variables"`
	SystemOverride bool     `json:"system_override"`
}

type HookItemDTO struct {
	ID      string `json:"id"`
	Event   string `json:"event"`
	Type    string `json:"type"` // "SHELL", "DOCKER", "WEBHOOK"
	Command string `json:"command"`
	Policy  string `json:"policy"` // "BLOCK", "WARN"
}

func (r *Router) handleRegistries(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	data := RegistryDTO{
		Skills: []SkillItemDTO{
			{ID: "bmad_architect", Name: "BMAD Architecture Reviewer", Source: "BUILTIN", Format: "BMAD", Description: "Analyzes system architecture and AST impact"},
			{ID: "claude_atdd_generator", Name: "Claude ATDD Generator", Source: "PROJECT_LOCAL", Format: "CLAUDE", Description: "Grounded test suite authoring from SKILL.md"},
			{ID: "superpower_container_runner", Name: "Superpower Shell Harness", Source: "USER_SYSTEM", Format: "SUPERPOWER", Description: "High-speed isolated execution with bounded capabilities"},
			{ID: "mcp_filesystem_bridge", Name: "MCP Host Bridge", Source: "REMOTE_GIT", Format: "MCP", Description: "Model Context Protocol bridge for container execution", RepoURL: "github.com/modelcontextprotocol/servers"},
		},
		Prompts: []PromptItemDTO{
			{ID: "prompt_architect_v3", Name: "Architect RFC Generator", Source: "PROJECT_LOCAL", Variables: []string{"system_architecture", "prd_requirements"}, SystemOverride: true},
			{ID: "prompt_qa_atdd_v2", Name: "QA ATDD Generator", Source: "BUILTIN", Variables: []string{"ast_routes", "acceptance_criteria"}, SystemOverride: false},
			{ID: "prompt_developer_bmad", Name: "BMAD Persona Implementation", Source: "USER_SYSTEM", Variables: []string{"task_wbs", "codebase_context"}, SystemOverride: true},
		},
		Hooks: []HookItemDTO{
			{ID: "pre_stage_security_check", Event: "pre-stage", Type: "SHELL", Command: "./scripts/security_audit.sh", Policy: "BLOCK"},
			{ID: "on_failure_slack_notify", Event: "on-failure", Type: "WEBHOOK", Command: "https://hooks.slack.com/services/T00/B00/X00", Policy: "WARN"},
			{ID: "pre_commit_linter", Event: "pre-commit", Type: "DOCKER", Command: "golangci-lint run", Policy: "BLOCK"},
		},
	}

	r.writeJSON(w, http.StatusOK, data)
}
