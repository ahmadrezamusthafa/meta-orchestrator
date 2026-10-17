package stack

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func writeRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func ids(p Profile) []string {
	var out []string
	for _, t := range p.Techs {
		out = append(out, t.ID)
	}
	return out
}

// goService is a Go + PostgreSQL API with OIDC, SpiceDB, OpenTelemetry, an OpenAPI contract and a
// container deployed to Azure Container Apps by GitHub Actions.
var goService = map[string]string{
	"go.mod": `module example.com/billing
require (
	github.com/jackc/pgx/v5 v5.0.0
	github.com/coreos/go-oidc/v3 v3.0.0
	github.com/authzed/authzed-go v1.0.0
	go.opentelemetry.io/otel v1.0.0
)`,
	".golangci.yml":                   "linters: {}",
	"api/openapi.yaml":                "openapi: 3.0.3\ninfo:\n  title: Billing\n",
	"authz/schema.zed":                "definition user {}",
	"Dockerfile":                      "FROM golang AS build",
	".github/workflows/deploy.yml":    "steps:\n  - uses: azure/container-apps-deploy-action@v1",
	"node_modules/react/package.json": `{"name":"react"}`, // dependencies are never scanned
	"vendor/x/go.mod":                 "module x",
}

// reactApp is a React + TypeScript portal using Keycloak, tested with Playwright, built with pnpm.
var reactApp = map[string]string{
	"package.json": `{"name":"portal","scripts":{"typecheck":"tsc -b","lint":"eslint .","test":"vitest","build":"vite build","test:e2e":"playwright test"},
	"dependencies":{"react":"^18.0.0","keycloak-js":"^25.0.0","@opentelemetry/api":"^1.0.0"},"devDependencies":{"typescript":"^5.0.0","@playwright/test":"^1.40.0"}}`,
	"pnpm-lock.yaml":       "lockfileVersion: '9.0'",
	"tsconfig.json":        "{}",
	"playwright.config.ts": "export default {}",
}

// infraLive is an OpenTofu + Terragrunt repository for Azure Container Apps, Cloudflare and Grafana.
var infraLive = map[string]string{
	"live/root.hcl":                    "remote_state {\n  backend = \"azurerm\"\n}",
	"live/prod/billing/terragrunt.hcl": "terraform {\n  source = \"../../../modules/container-app\"\n}",
	"modules/container-app/main.tf":    `resource "azurerm_container_app" "this" {}`,
	"modules/edge/main.tf":             `terraform { required_providers { cloudflare = { source = "cloudflare/cloudflare" } } }`,
	"grafana/dashboards/billing.json":  `{"panels": [], "schemaVersion": 39}`,
}

func TestDetectGoService(t *testing.T) {
	p := Detect(writeRepo(t, goService), Deep)
	want := []string{"go", "postgresql", "keycloak", "spicedb", "azure-container-apps", "github-actions", "docker", "opentelemetry", "openapi"}
	if got := ids(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("techs = %v, want %v", got, want)
	}
	if goTech, _ := p.Get("go"); !reflect.DeepEqual(goTech.Dirs, []string{"."}) {
		t.Errorf("vendored modules must not count: go dirs = %v", goTech.Dirs)
	}
}

func TestDetectReactApp(t *testing.T) {
	p := Detect(writeRepo(t, reactApp), Deep)
	want := []string{"react", "typescript", "keycloak", "playwright", "opentelemetry"}
	if got := ids(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("techs = %v, want %v", got, want)
	}
	if m := p.nodePkgs["."].manager; m != "pnpm" {
		t.Errorf("package manager = %q, want pnpm from the lockfile", m)
	}
}

func TestDetectInfra(t *testing.T) {
	p := Detect(writeRepo(t, infraLive), Deep)
	want := []string{"azure-container-apps", "cloudflare", "opentofu", "terragrunt", "grafana"}
	if got := ids(p); !reflect.DeepEqual(got, want) {
		t.Fatalf("techs = %v, want %v", got, want)
	}
	if s := p.Surface(); s != "" {
		t.Errorf("infrastructure has no UAT surface, got %q", s)
	}
}

func TestDetectMonorepoAndLimits(t *testing.T) {
	root := writeRepo(t, map[string]string{
		"go.mod":           "module m",
		"web/package.json": `{"packageManager":"yarn@4.0.0","dependencies":{"react":"18"},"scripts":{"lint":"eslint"}}`,
	})
	p := Detect(root, Deep)
	react, ok := p.Get("react")
	if !ok || react.AtRoot() || react.Dirs[0] != "web" {
		t.Fatalf("react should be found in web/, got %+v", react)
	}
	if m := p.nodePkgs["web"].manager; m != "yarn" {
		t.Errorf("packageManager field should win, got %q", m)
	}
	if shallow := Detect(root, Options{MaxDepth: 0, MaxFiles: 100}); shallow.Has("react") {
		t.Error("a root-only scan must not see web/")
	}
	if capped := Detect(writeRepo(t, goService), Options{MaxDepth: 4, MaxFiles: 2}); !capped.Truncated {
		t.Error("hitting the file cap must mark the profile truncated")
	}
	if empty := For(filepath.Join(root, "missing")); len(empty.Techs) != 0 {
		t.Error("a missing folder has an empty profile")
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name     string
		files    map[string]string
		role     types.ProjectRole
		manifest string
	}{
		{"billing", goService, types.RoleBackend, "go.mod"},
		// A React app with its own Playwright config is a frontend, not a test suite.
		{"portal", reactApp, types.RoleFrontend, "package.json"},
		{"platform-live", infraLive, types.RoleInfrastructure, "root.hcl"},
		{"billing-contracts", map[string]string{"openapi.yaml": "openapi: 3.1.0"}, types.RoleContracts, "openapi.yaml"},
		{"qa-suite", map[string]string{"package.json": `{"devDependencies":{"@playwright/test":"1"}}`, "playwright.config.ts": ""}, types.RoleAutomationTest, "playwright.config.ts"},
		{"frontend-app", map[string]string{"package.json": `{"name":"fe"}`}, types.RoleFrontend, "package.json"},
		{"rails-app", map[string]string{"Gemfile": "gem 'rails'"}, types.RoleBackend, "Gemfile"},
		// A server app bundling its own screens stays a backend.
		{"billing", map[string]string{"Gemfile": "gem 'rails'", "package.json": `{"dependencies":{"vue":"3"}}`}, types.RoleBackend, "Gemfile"},
		{"billing-internal-fe", map[string]string{"package.json": `{"dependencies":{"vue":"3"}}`}, types.RoleFrontend, "package.json"},
		{"notes", map[string]string{"readme.txt": "hi"}, types.RoleOtherService, Unknown},
	}
	for _, c := range cases {
		got := Classify(c.name, writeRepo(t, c.files))
		if got.Role != c.role || got.Manifest != c.manifest {
			t.Errorf("%s: got %s/%s, want %s/%s", c.name, got.Role, got.Manifest, c.role, c.manifest)
		}
	}
}

func TestSurface(t *testing.T) {
	if s := Detect(writeRepo(t, goService), Deep).Surface(); s != "api" {
		t.Errorf("go service surface = %q, want api", s)
	}
	if s := Detect(writeRepo(t, reactApp), Deep).Surface(); s != "web" {
		t.Errorf("react app surface = %q, want web", s)
	}
	if s := (Profile{}).Surface(); s != "web" {
		t.Errorf("unknown stack keeps the web default, got %q", s)
	}
}

func TestBrief(t *testing.T) {
	brief := Brief([]Repo{
		{Name: "billing", Profile: Detect(writeRepo(t, goService), Deep)},
		{Name: "portal", Profile: Detect(writeRepo(t, reactApp), Deep)},
		{Name: "infra", Profile: Detect(writeRepo(t, infraLive), Deep)},
		{Name: "empty", Profile: Profile{}},
	})
	for _, want := range []string{
		"- billing: Go, PostgreSQL, Keycloak (OIDC), SpiceDB",
		"- billing: `go build ./... && go vet ./... && go test ./...`",
		"- billing: `golangci-lint run` (if installed)",
		"- billing: `npx @redocly/cli lint api/openapi.yaml`",
		"- billing: `zed validate authz/schema.zed` (if zed is installed)",
		"- billing: `actionlint` (if installed)",
		"- portal: `pnpm run typecheck`",
		"- portal: `CI=1 pnpm run test`",
		"- portal: `pnpm run test:e2e` — only the specs your change touches",
		"- infra/modules: `tofu fmt -check -recursive`",
		"- infra/live: `terragrunt hclfmt --check`",
		"Azure Container Apps: change apps through the repository's IaC",
		"`tofu apply|destroy|import|state …|force-unlock`",
		"`az containerapp create|update|up|delete`",
		"`wrangler deploy|publish|delete`",
		"`zed schema write`",
	} {
		if !strings.Contains(brief, want) {
			t.Errorf("brief is missing %q\n%s", want, brief)
		}
	}
	if strings.Contains(brief, "- empty:") {
		t.Error("a repository with no detected stack is left out")
	}
	if strings.Count(brief, "OpenTelemetry: use the existing") != 1 {
		t.Error("a convention shared by several repositories is listed once")
	}
	if Brief([]Repo{{Name: "x"}}) != "" {
		t.Error("no detected stack means no brief")
	}
}
