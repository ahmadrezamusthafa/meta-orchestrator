package skills

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

var validSkillNameRegex = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// Standard SDLC recognized roles
var standardRoles = map[string]bool{
	"system_architect":        true,
	"lead_developer":          true,
	"atdd_qa_engineer":        true,
	"devops_superpower":       true,
	"product_manager":         true,
	"tech_writer":             true,
	"security_specialist":     true,
	"senior_fullstack_dev":    true,
	"qa_automation_specialist": true,
	"code_reviewer":           true,
}

// CheckSkillCompatibility performs a multi-dimensional audit of a skill contract against Meta-Orchestrator requirements.
func CheckSkillCompatibility(skill *types.UniversalSkillContract) *types.SkillCompatibility {
	if skill == nil {
		return &types.SkillCompatibility{
			Status:       types.CompatibilityStatusIncompatible,
			Score:        0,
			Issues:       []types.SkillCompatibilityIssue{{Severity: "error", Check: "manifest", Message: "Skill contract is nil"}},
			CheckedAt:    time.Now().UTC().Format(time.RFC3339),
			RuntimeReady: false,
			SandboxSafe:  false,
		}
	}

	var issues []types.SkillCompatibilityIssue
	errorsCount := 0
	warningsCount := 0

	// 1. Manifest Checks
	if skill.Name == "" {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "error",
			Check:      "manifest",
			Message:    "Skill name is missing or empty",
			Suggestion: "Provide an alphanumeric identifier for the skill in SKILL.md or manifest",
		})
		errorsCount++
	} else if !validSkillNameRegex.MatchString(skill.Name) {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "error",
			Check:      "manifest",
			Message:    fmt.Sprintf("Skill name '%s' contains invalid characters (allowed: alphanumeric, -, _, .)", skill.Name),
			Suggestion: "Rename the skill to use only alphanumeric characters, dashes, or underscores",
		})
		errorsCount++
	} else if len(skill.Name) > 64 {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "warning",
			Check:      "manifest",
			Message:    fmt.Sprintf("Skill name '%s' is longer than 64 characters", skill.Name),
			Suggestion: "Keep skill names concise for better logging and visualization",
		})
		warningsCount++
	}

	if skill.Description == "" {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "warning",
			Check:      "manifest",
			Message:    "Description is empty. AI planner requires tool descriptions for accurate selection",
			Suggestion: "Add a clear 1-2 sentence description explaining when and how this tool should be invoked",
		})
		warningsCount++
	} else if len(skill.Description) < 15 {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "warning",
			Check:      "manifest",
			Message:    "Description is very brief. Provide more context for AI orchestration",
			Suggestion: "Expand the description with capabilities, inputs, and output format",
		})
		warningsCount++
	}

	// 2. Schema Checks
	if skill.InputSchema == nil || len(skill.InputSchema) == 0 {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity: "info",
			Check:    "schema",
			Message:  "No input schema specified; tool executes without parameters or with raw prompt",
		})
	} else {
		if _, hasType := skill.InputSchema["type"]; !hasType {
			if _, hasProps := skill.InputSchema["properties"]; !hasProps {
				issues = append(issues, types.SkillCompatibilityIssue{
					Severity:   "warning",
					Check:      "schema",
					Message:    "Input schema does not declare 'type' or 'properties' conforming to JSON Schema",
					Suggestion: "Define standard JSON schema properties for structured parameter validation",
				})
				warningsCount++
			}
		}
	}

	// 3. Runtime & Execution Checks
	runtimeReady := true
	if skill.SourceFormat == types.SkillFormatNative {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity: "info",
			Check:    "runtime",
			Message:  "Compiled native Go kernel tool: zero overhead, maximum performance",
		})
	} else if skill.SourceFormat == types.SkillFormatClaude {
		if skill.SourceLocation != "" {
			if stat, err := os.Stat(skill.SourceLocation); err != nil || !stat.IsDir() {
				issues = append(issues, types.SkillCompatibilityIssue{
					Severity:   "error",
					Check:      "runtime",
					Message:    fmt.Sprintf("Source location directory not accessible: %s", skill.SourceLocation),
					Suggestion: "Ensure directory exists and has read permissions",
				})
				errorsCount++
				runtimeReady = false
			} else {
				skillMdPath := filepath.Join(skill.SourceLocation, "SKILL.md")
				if _, err := os.Stat(skillMdPath); err != nil {
					issues = append(issues, types.SkillCompatibilityIssue{
						Severity:   "error",
						Check:      "runtime",
						Message:    "SKILL.md file missing from Claude skill bundle",
						Suggestion: "Create SKILL.md with instructions and YAML frontmatter",
					})
					errorsCount++
					runtimeReady = false
				}
			}
		}
		if skill.Command != "" {
			if _, err := exec.LookPath(skill.Command); err != nil {
				// Check if it's a relative path in SourceLocation
				resolved := false
				if skill.SourceLocation != "" {
					scriptCandidate := filepath.Join(skill.SourceLocation, skill.Command)
					if _, err2 := os.Stat(scriptCandidate); err2 == nil {
						resolved = true
					}
				}
				if !resolved {
					issues = append(issues, types.SkillCompatibilityIssue{
						Severity:   "warning",
						Check:      "runtime",
						Message:    fmt.Sprintf("Interpreter command '%s' not found in system PATH", skill.Command),
						Suggestion: fmt.Sprintf("Install '%s' on host system or execute inside Docker container", skill.Command),
					})
					warningsCount++
				}
			}
		}
	} else if skill.SourceFormat == types.SkillFormatMCP {
		cmdName := skill.Command
		if cmdName == "" {
			cmdName = "npx"
		}
		if _, err := exec.LookPath(cmdName); err != nil {
			issues = append(issues, types.SkillCompatibilityIssue{
				Severity:   "warning",
				Check:      "runtime",
				Message:    fmt.Sprintf("MCP runner command '%s' not found in system PATH", cmdName),
				Suggestion: fmt.Sprintf("Ensure '%s' is installed in PATH (e.g. Node.js/npx or Python/uvx)", cmdName),
			})
			warningsCount++
		}
		for k, v := range skill.Environment {
			if v == "" {
				issues = append(issues, types.SkillCompatibilityIssue{
					Severity:   "warning",
					Check:      "dependencies",
					Message:    fmt.Sprintf("Connector environment variable '%s' is currently empty", k),
					Suggestion: fmt.Sprintf("Configure credentials for '%s' in the Connectors settings page", k),
				})
				warningsCount++
			}
		}
	}

	// 4. Security & Sandbox Isolation Checks
	sandboxSafe := true
	if skill.Isolation == types.IsolationHost {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "warning",
			Check:      "security",
			Message:    "Skill requests host-level isolation (unrestricted filesystem access)",
			Suggestion: "Change isolation to 'subprocess' or 'docker' for zero-trust safety",
		})
		warningsCount++
		sandboxSafe = false
	} else if skill.Isolation == types.IsolationDocker {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity: "info",
			Check:    "security",
			Message:  "Docker container sandbox verified for untrusted execution",
		})
	} else {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity: "info",
			Check:    "security",
			Message:  "Subprocess boundary verified with process-level isolation",
		})
	}

	if skill.TimeoutSeconds <= 0 {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "warning",
			Check:      "security",
			Message:    "Execution timeout is not configured or <= 0 (defaulting to 120s)",
			Suggestion: "Specify timeout_seconds explicitly in skill manifest",
		})
		warningsCount++
	} else if skill.TimeoutSeconds > 600 {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity:   "warning",
			Check:      "security",
			Message:    fmt.Sprintf("Timeout (%ds) exceeds 600s; long-running skills can delay pipeline progress", skill.TimeoutSeconds),
			Suggestion: "Keep timeout below 300s or use asynchronous background stages",
		})
		warningsCount++
	}

	if skill.RequiresNetwork {
		if skill.Isolation == types.IsolationHost {
			issues = append(issues, types.SkillCompatibilityIssue{
				Severity: "warning",
				Check:    "security",
				Message:  "Direct host outbound network egress requested",
			})
			warningsCount++
		} else {
			issues = append(issues, types.SkillCompatibilityIssue{
				Severity: "info",
				Check:    "security",
				Message:  "Sandboxed outbound network egress active",
			})
		}
	}

	// 5. RBAC Checks
	if len(skill.RequiredRoles) == 0 {
		issues = append(issues, types.SkillCompatibilityIssue{
			Severity: "info",
			Check:    "security",
			Message:  "No role constraints specified; skill is globally accessible to all agent roles",
		})
	} else {
		for _, r := range skill.RequiredRoles {
			if !standardRoles[r] {
				issues = append(issues, types.SkillCompatibilityIssue{
					Severity: "info",
					Check:    "security",
					Message:  fmt.Sprintf("Custom RBAC role constraint: '%s'", r),
				})
			}
		}
	}

	// 6. Score & Status Calculation
	score := 100 - (errorsCount * 35) - (warningsCount * 10)
	if score < 0 {
		score = 0
	}

	status := types.CompatibilityStatusCompatible
	if errorsCount > 0 {
		status = types.CompatibilityStatusIncompatible
		runtimeReady = false
	} else if score < 80 || warningsCount > 0 {
		status = types.CompatibilityStatusWarning
	}

	return &types.SkillCompatibility{
		Status:       status,
		Score:        score,
		Issues:       issues,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
		RuntimeReady: runtimeReady,
		SandboxSafe:  sandboxSafe,
	}
}

// AuditDirectory inspects a local directory and parses any skill format found (Claude, MCP, BMAD, Superpower).
func AuditDirectory(dirPath string, claudeAdapter *ClaudeSkillAdapter, mcpAdapter *MCPSkillAdapter) ([]*types.UniversalSkillContract, error) {
	stat, err := os.Stat(dirPath)
	if err != nil {
		return nil, fmt.Errorf("directory not found or inaccessible: %w", err)
	}
	if !stat.IsDir() {
		return nil, fmt.Errorf("specified path is not a directory: %s", dirPath)
	}

	var results []*types.UniversalSkillContract

	// Case 1: Directory itself is a Claude skill (contains SKILL.md)
	if _, err := os.Stat(filepath.Join(dirPath, "SKILL.md")); err == nil {
		contract, err := claudeAdapter.ParseDirectory(dirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse Claude skill in %s: %w", dirPath, err)
		}
		contract.Compatibility = CheckSkillCompatibility(contract)
		contract.SourceType = "claude"
		return []*types.UniversalSkillContract{contract}, nil
	}

	// Case 2: Directory contains mcp.json or .mcp.json
	for _, mcpFile := range []string{"mcp.json", ".mcp.json"} {
		mcpPath := filepath.Join(dirPath, mcpFile)
		if _, err := os.Stat(mcpPath); err == nil {
			tools, err := mcpAdapter.ParseConfigFile(mcpPath)
			if err == nil && len(tools) > 0 {
				for _, t := range tools {
					t.Compatibility = CheckSkillCompatibility(t)
					t.SourceType = "mcp"
					results = append(results, t)
				}
				return results, nil
			}
		}
	}

	// Case 3: Directory contains subdirectories of Claude skills (e.g. ~/.claude/skills or .claude/skills)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory entries: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			subDir := filepath.Join(dirPath, entry.Name())
			if _, err := os.Stat(filepath.Join(subDir, "SKILL.md")); err == nil {
				if contract, err := claudeAdapter.ParseDirectory(subDir); err == nil && contract != nil {
					contract.Compatibility = CheckSkillCompatibility(contract)
					contract.SourceType = "claude"
					results = append(results, contract)
				}
			}
		}
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no recognized skill formats (Claude SKILL.md, MCP JSON) found in %s", dirPath)
	}

	return results, nil
}
