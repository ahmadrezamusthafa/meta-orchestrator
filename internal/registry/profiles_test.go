package registry

import (
	"path/filepath"
	"testing"
)

func TestProfileRegistryAndRBAC(t *testing.T) {
	reg := NewProfileRegistry()
	profilesPath := filepath.Join("..", "..", "configs", "profiles.json")

	err := reg.LoadFromFile(profilesPath)
	if err != nil {
		t.Fatalf("failed to load profiles: %v", err)
	}

	// 1. Check Product Manager permissions
	allowed, err := reg.CheckPermission("product_manager", "read_ast_node")
	if err != nil || !allowed {
		t.Errorf("expected read_ast_node to be allowed for product_manager")
	}

	allowed, err = reg.CheckPermission("product_manager", "docker_compose_up")
	if err != nil || allowed {
		t.Errorf("expected docker_compose_up to be DENIED for product_manager")
	}

	// 2. Check ATDD QA Engineer cannot write source code directly
	allowed, err = reg.CheckPermission("atdd_qa_engineer", "write_source_code")
	if err != nil || allowed {
		t.Errorf("expected write_source_code to be DENIED for atdd_qa_engineer")
	}

	// 3. Check Lead Developer can write source code
	allowed, err = reg.CheckPermission("lead_developer", "write_source_code")
	if err != nil || !allowed {
		t.Errorf("expected write_source_code to be ALLOWED for lead_developer")
	}

	// 4. Check DevOps superpower has broad access
	allowed, err = reg.CheckPermission("devops_superpower", "docker_compose_up")
	if err != nil || !allowed {
		t.Errorf("expected docker_compose_up to be ALLOWED for devops_superpower")
	}
}

func TestProfileIsDeniedOnlyHonoursExplicitDenies(t *testing.T) {
	reg := NewProfileRegistry()
	if err := reg.LoadFromFile(filepath.Join("..", "..", "configs", "profiles.json")); err != nil {
		t.Fatal(err)
	}
	if !reg.IsDenied("lead_developer", "execute_shell_host") {
		t.Error("an explicit deny must be reported")
	}
	if reg.IsDenied("lead_developer", "bmad-dev-story") {
		t.Error("a skill absent from allowed_skills is not denied")
	}
	if reg.IsDenied("no_such_role", "anything") {
		t.Error("an unknown role denies nothing")
	}
}
