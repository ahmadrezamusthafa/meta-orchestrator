package api

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/projects"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestStackBriefDescribesAssignedRepos(t *testing.T) {
	root, svc, infra := t.TempDir(), t.TempDir(), t.TempDir()
	_ = os.WriteFile(filepath.Join(svc, "go.mod"), []byte("module svc\nrequire github.com/jackc/pgx/v5 v5.0.0"), 0o644)
	_ = os.WriteFile(filepath.Join(infra, "terragrunt.hcl"), []byte("terraform {}"), 0o644)
	_ = os.WriteFile(filepath.Join(infra, "main.tf"), []byte(`resource "azurerm_container_app" "a" {}`), 0o644)

	pm := projects.NewProjectManager(root)
	p, err := pm.Create(&types.Project{Name: "Billing", Repos: []types.ProjectRepo{{Name: "svc", Path: svc}, {Name: "infra", Path: infra}}})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if got := p.Repos[0].Stack; !reflect.DeepEqual(got, []string{"Go", "PostgreSQL"}) {
		t.Errorf("registered repo stack = %v", got)
	}
	r := NewRouter(RouterConfig{RootDir: root, ProjectManager: pm})

	brief := r.stackBrief(&types.Task{AssignedRepos: []string{"svc", "infra", "unregistered"}})
	for _, want := range []string{"- svc: Go, PostgreSQL", "- infra: Azure Container Apps, OpenTofu, Terragrunt",
		"`go build ./... && go vet ./... && go test ./...`", "`tofu apply|destroy|import|state …|force-unlock`"} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief is missing %q:\n%s", want, brief)
		}
	}
	if r.stackBrief(&types.Task{}) != "" {
		t.Error("a task without repositories gets no stack brief")
	}

	labels := r.repoLabels([]string{"infra", "unregistered"})
	if want := []string{"infra (Azure Container Apps, OpenTofu, Terragrunt)", "unregistered"}; !reflect.DeepEqual(labels, want) {
		t.Errorf("repoLabels = %v, want %v", labels, want)
	}
}
