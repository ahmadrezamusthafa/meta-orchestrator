package registry

import (
	"path/filepath"
	"testing"
)

func TestMethodRegistry(t *testing.T) {
	reg := NewMethodRegistry()
	methodsPath := filepath.Join("..", "..", "configs", "methods.json")

	err := reg.LoadFromFile(methodsPath)
	if err != nil {
		t.Fatalf("failed to load methods: %v", err)
	}

	// 1. Verify BMAD method
	bmad, err := reg.Get("bmad")
	if err != nil || bmad == nil {
		t.Fatalf("expected bmad to exist: %v", err)
	}
	if bmad.ExecutionPattern != "sequential" {
		t.Errorf("expected bmad sequential pattern, got %s", bmad.ExecutionPattern)
	}

	// 2. Role validation in BMAD
	ok, _ := reg.ValidateRole("bmad", "atdd_qa_engineer")
	if !ok {
		t.Errorf("expected atdd_qa_engineer allowed in bmad")
	}

	// 3. Superpower method
	sp, err := reg.Get("superpower")
	if err != nil || sp == nil {
		t.Fatalf("expected superpower to exist: %v", err)
	}
	if sp.ExecutionPattern != "autonomous_infrastructure" {
		t.Errorf("expected autonomous_infrastructure, got %s", sp.ExecutionPattern)
	}
}
