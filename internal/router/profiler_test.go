package router

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
)

func TestRepoDiscoveryAndTaskProfiler(t *testing.T) {
	tmpWorkspace := t.TempDir()

	// 1. Create simulated multi-repo structure:
	// - frontend-portal (package.json)
	// - core-backend-api (go.mod)
	frontendDir := filepath.Join(tmpWorkspace, "frontend-portal")
	backendDir := filepath.Join(tmpWorkspace, "core-backend-api")
	_ = os.MkdirAll(frontendDir, 0755)
	_ = os.MkdirAll(backendDir, 0755)

	_ = os.WriteFile(filepath.Join(frontendDir, "package.json"), []byte(`{"name": "frontend-portal"}`), 0644)
	_ = os.WriteFile(filepath.Join(backendDir, "go.mod"), []byte(`module github.com/org/core-backend-api`), 0644)

	cfg := config.GetDefaultConfig()
	profiler := NewTaskProfiler(cfg, tmpWorkspace)

	// 2. Test cross-repo feature request profiling
	profile, err := profiler.ProfileTask(
		"Implement User Checkout Flow",
		"Build responsive checkout page in UI and wire to backend payments API",
		"",
	)
	if err != nil {
		t.Fatalf("failed to profile task: %v", err)
	}

	if profile.WorkflowID != "general-ai-sdlc" {
		t.Errorf("expected workflow 'general-ai-sdlc', got '%s'", profile.WorkflowID)
	}
	if len(profile.IdentifiedRepos) != 2 {
		t.Errorf("expected both frontend and backend repos identified, got %v", profile.IdentifiedRepos)
	}

	// 3. Test hotfix emergency request workflow resolution
	hotfixProfile, err := profiler.ProfileTask(
		"Emergency Hotfix: Null pointer in checkout",
		"Critical hotfix patch for production payment gateway error",
		"",
	)
	if err != nil {
		t.Fatalf("failed to profile hotfix: %v", err)
	}
	if hotfixProfile.WorkflowID != "hotfix-fast-track" {
		t.Errorf("expected workflow 'hotfix-fast-track', got '%s'", hotfixProfile.WorkflowID)
	}

	// 4. Test low complexity classification
	lowProfile, _ := profiler.ProfileTask("Fix typo in README", "Correct misspelling in docs", "")
	if lowProfile.Complexity != "LOW" {
		t.Errorf("expected LOW complexity, got %s", lowProfile.Complexity)
	}
}
