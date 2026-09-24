package docker

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestDockerComposeGenerationAndSkillMounts(t *testing.T) {
	tmpDir := t.TempDir()

	testSkills := []*types.UniversalSkillContract{
		{
			Name:           "custom_claude_skill",
			SourceFormat:   types.SkillFormatClaude,
			SourceLocation: filepath.Join(tmpDir, "claude-skill"),
		},
		{
			Name:           "superpower_shell",
			SourceFormat:   types.SkillFormatSuperpower,
			SourceLocation: filepath.Join(tmpDir, "superpower-tool"),
		},
	}

	mounts := GenerateSkillMounts(testSkills, "task-doc-1")
	if len(mounts) < 3 { // 2 skills + 1 artifacts bridge
		t.Fatalf("expected at least 3 mounts, got %d", len(mounts))
	}

	orch := NewComposeOrchestrator()
	composePath, err := orch.GenerateComposeFile("task-doc-1", tmpDir, mounts, []string{"postgres", "redis"})
	if err != nil {
		t.Fatalf("failed to generate compose file: %v", err)
	}

	data, err := os.ReadFile(composePath)
	if err != nil {
		t.Fatalf("failed to read generated compose file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "app_task-doc-1") {
		t.Errorf("missing app_sandbox container")
	}
	if !strings.Contains(content, "pg_task-doc-1") {
		t.Errorf("missing postgres service")
	}
	if !strings.Contains(content, "/opt/skills/custom_claude_skill") {
		t.Errorf("missing Claude skill mount")
	}

	// Healthcheck
	hc := NewServiceHealthChecker()
	err = hc.WaitForHealthy(context.Background(), []string{"postgres", "redis"}, 1*time.Second)
	if err != nil {
		t.Fatalf("healthcheck failed: %v", err)
	}
}
