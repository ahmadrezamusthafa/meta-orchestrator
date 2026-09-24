package artifacts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArtifactManagerWriteAndValidate(t *testing.T) {
	tmpDir := t.TempDir()
	mgr := NewArtifactManager(tmpDir)

	taskID := "task-art-101"

	// 1. Invalid PRD without required heading
	_, err := mgr.WriteArtifact(taskID, ArtifactPRD, "Random unstructured content", "product_manager")
	if err == nil {
		t.Fatalf("expected validation error for malformed PRD")
	}

	// 2. Valid PRD
	prdContent := `# Product Requirements Document: User Authentication
## 1. Overview
Secure passwordless authentication using WebAuthn.
`
	meta, err := mgr.WriteArtifact(taskID, ArtifactPRD, prdContent, "product_manager")
	if err != nil {
		t.Fatalf("failed to write PRD artifact: %v", err)
	}

	if meta.Type != ArtifactPRD {
		t.Errorf("expected type PRD.md, got %s", meta.Type)
	}
	if meta.SHA256Hash == "" {
		t.Errorf("expected non-empty SHA256 hash")
	}

	// Verify file on disk
	expectedFile := filepath.Join(tmpDir, taskID, "PRD.md")
	data, err := os.ReadFile(expectedFile)
	if err != nil {
		t.Fatalf("file not created on disk: %v", err)
	}
	if string(data) != prdContent {
		t.Errorf("content on disk does not match")
	}

	// 3. Read back
	readContent, readMeta, err := mgr.ReadArtifact(taskID, ArtifactPRD)
	if err != nil || readMeta == nil {
		t.Fatalf("failed to read artifact: %v", err)
	}
	if readContent != prdContent {
		t.Errorf("read content mismatch")
	}

	// 4. Valid ATDD Suite
	atddContent := `# Acceptance Test Suite (ATDD)
Feature: Authentication Flow
  Scenario: User clicks magic link
    Given the user is on the login page
`
	_, err = mgr.WriteArtifact(taskID, ArtifactATDDSuite, atddContent, "atdd_qa_engineer")
	if err != nil {
		t.Fatalf("failed to write ATDD artifact: %v", err)
	}
}

func TestArtifactValidator(t *testing.T) {
	v := NewArtifactValidator()

	// 1. Incomplete PRD
	errs := v.ValidatePRD("# PRD: Some title\nNo user stories here")
	if len(errs) == 0 {
		t.Errorf("expected validation errors for PRD missing user stories")
	}

	// 2. Complete PRD
	errs = v.ValidatePRD("# Product Requirements\n## Acceptance Criteria\n- AC1: pass")
	if len(errs) != 0 {
		t.Errorf("expected no errors for valid PRD, got %v", errs)
	}

	// 3. ATDD validator
	errs = v.ValidateATDD("Feature: Order Payment\nScenario: Successful payment")
	if len(errs) != 0 {
		t.Errorf("expected valid ATDD, got %v", errs)
	}
}
