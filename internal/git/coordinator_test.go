package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMultiRepoGitCoordinator(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize 2 real git repos
	repo1 := filepath.Join(tmpDir, "repo-frontend")
	repo2 := filepath.Join(tmpDir, "repo-backend")
	_ = os.MkdirAll(repo1, 0755)
	_ = os.MkdirAll(repo2, 0755)

	for _, p := range []string{repo1, repo2} {
		_ = exec.Command("git", "-C", p, "init").Run()
		_ = exec.Command("git", "-C", p, "config", "user.email", "test@meta.io").Run()
		_ = exec.Command("git", "-C", p, "config", "user.name", "MetaTest").Run()
		_ = os.WriteFile(filepath.Join(p, "README.md"), []byte("# Initial"), 0644)
		_ = exec.Command("git", "-C", p, "add", ".").Run()
		_ = exec.Command("git", "-C", p, "commit", "-m", "initial commit").Run()
	}

	coord := NewMultiRepoGitCoordinator()
	repoMap := map[string]string{
		"repo-frontend": repo1,
		"repo-backend":  repo2,
	}

	// 1. Format Branch Name
	branch := coord.FormatBranchName("TASK-101", "Order Checkout")
	if branch != "feat/TASK-101-order-checkout" {
		t.Errorf("unexpected branch name: %s", branch)
	}

	// 2. Create Coordinated Branch
	checkpoints, err := coord.CreateCoordinatedBranch(repoMap, branch)
	if err != nil || len(checkpoints) != 2 {
		t.Fatalf("failed to create coordinated branch: %v", err)
	}

	// Verify both repos on branch
	for _, p := range []string{repo1, repo2} {
		out, _ := exec.Command("git", "-C", p, "rev-parse", "--abbrev-ref", "HEAD").CombinedOutput()
		if strings.TrimSpace(string(out)) != branch {
			t.Errorf("repo %s not on branch %s (got %s)", p, branch, string(out))
		}
	}

	// 3. PR Draft Generation
	pr := coord.GeneratePRDraft("TASK-101", "Order Checkout Flow", branch, []string{"repo-frontend", "repo-backend"}, "mock-sha-256")
	if pr.Title != "[TASK-101] Order Checkout Flow" {
		t.Errorf("unexpected PR title: %s", pr.Title)
	}
	if !strings.Contains(pr.Body, "mock-sha-256") {
		t.Errorf("PR body missing SHA signature")
	}
}
