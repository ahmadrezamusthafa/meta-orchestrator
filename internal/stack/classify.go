package stack

import (
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Unknown is the manifest of a folder with nothing recognisable in it.
const Unknown = "unknown"

// appManifests mark an application at a repository root, in the order they name it.
var appManifests = []string{"go.mod", "package.json", "composer.json", "Gemfile", "pom.xml", "build.gradle.kts", "build.gradle",
	"pubspec.yaml", "Cargo.toml", "requirements.txt", "pyproject.toml"}

// Classification is what the project wizard suggests for a folder.
type Classification struct {
	Role     types.ProjectRole
	Manifest string   // the file that decided the role, or Unknown
	Stack    []string // detected technologies, for display
}

// Classify suggests a role for the folder at dir (named name) from a quick scan of its files,
// falling back to its name.
func Classify(name, dir string) Classification {
	p := Detect(dir, Quick)
	c := Classification{Role: types.RoleOtherService, Manifest: Unknown, Stack: p.Names()}
	lower := strings.ToLower(name)
	tokens := map[string]bool{}
	for _, tok := range strings.FieldsFunc(lower, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
		tokens[tok] = true
	}
	// Short hints must be a whole word ("qa-suite" is not a "ui"); longer ones may be part of one.
	nameHas := func(words ...string) bool {
		for _, w := range words {
			if (len(w) <= 3 && tokens[w]) || (len(w) > 3 && strings.Contains(lower, w)) {
				return true
			}
		}
		return false
	}
	rootFile := func(names ...string) string {
		for _, n := range names {
			if st, err := os.Stat(filepath.Join(dir, n)); err == nil && !st.IsDir() {
				return n
			}
		}
		return ""
	}
	app := rootFile(appManifests...)
	// A frontend package beside a server manifest (a Rails or Go app bundling its own screens) is a
	// backend unless the folder's name says otherwise.
	server := rootFile("go.mod", "Gemfile", "composer.json", "pom.xml", "build.gradle.kts", "build.gradle", "pubspec.yaml",
		"Cargo.toml", "requirements.txt", "pyproject.toml")
	frontendName := nameHas("front", "fe", "ui", "web", "client", "portal")
	frontend := rootFile("package.json") != "" && (frontendName || (server == "" && isFrontendPackage(filepath.Join(dir, "package.json"))))
	if server != "" && !frontendName {
		app = server
	}
	set := func(role types.ProjectRole, manifest string) Classification {
		c.Role = role
		if manifest != "" {
			c.Manifest = manifest
		} else if app != "" {
			c.Manifest = app
		}
		return c
	}

	// 1. A test suite: an e2e config without an app of its own, or a test-named folder.
	if e2e := rootFile("playwright.config.ts", "playwright.config.js", "playwright.config.mjs", "cypress.config.ts", "cypress.config.js"); e2e != "" && !frontend && !p.has("go", true) {
		return set(types.RoleAutomationTest, e2e)
	}
	if nameHas("test", "e2e", "atdd", "automation") {
		return set(types.RoleAutomationTest, firstNonEmpty(rootFile("playwright.config.ts", "playwright.config.js"), app))
	}

	// 2. API contracts: a spec without a service around it, or a contract-named folder.
	if t, ok := p.Get("openapi"); ok && t.AtRoot() && app == "" {
		return set(types.RoleContracts, path.Base(t.Files[0]))
	}
	if nameHas("contract", "schema", "proto") {
		return set(types.RoleContracts, firstNonEmpty(rootFile("openapi.yaml", "openapi.yml", "openapi.json"), app))
	}

	// 3. Documentation and artifacts.
	if nameHas("artifact", "doc", "docs", "documentation", "prd", "spec") && app == "" {
		return set(types.RoleArtifact, "markdown")
	}

	// 4. Applications at the root. Mobile apps are frontends: a Flutter app, or an Android app whose
	// module (app/) sits right under the root.
	if p.has("flutter", true) {
		return set(types.RoleFrontend, "pubspec.yaml")
	}
	if t, ok := p.Get("android"); ok && depth(t.Dirs[0]) <= 1 {
		return set(types.RoleFrontend, firstNonEmpty(rootFile("settings.gradle.kts", "settings.gradle", "build.gradle.kts", "build.gradle"), path.Base(t.Files[0])))
	}
	if frontend {
		return set(types.RoleFrontend, "package.json")
	}
	if app != "" {
		return set(types.RoleBackend, app)
	}

	// 5. Infrastructure: IaC with no application around it.
	for _, id := range []string{"terragrunt", "opentofu", "cloudflare", "azure-container-apps"} {
		if t, ok := p.Get(id); ok {
			return set(types.RoleInfrastructure, path.Base(t.Files[0]))
		}
	}

	// 6. Applications in sub-folders (a monorepo), then the folder's name.
	switch {
	case p.has("go", false):
		t, _ := p.Get("go")
		return set(types.RoleBackend, t.Files[0])
	case p.has("java", false), p.has("kotlin", false):
		return set(types.RoleBackend, firstJVMBuild(p))
	case p.has("react", false), p.has("flutter", false):
		for _, id := range []string{"react", "flutter"} {
			if t, ok := p.Get(id); ok {
				return set(types.RoleFrontend, t.Files[0])
			}
		}
	case nameHas("infra", "terraform", "tofu", "terragrunt", "platform", "iac"):
		return set(types.RoleInfrastructure, "")
	case nameHas("service", "api", "backend"):
		return set(types.RoleBackend, "")
	}
	if len(p.Techs) > 0 && c.Manifest == Unknown {
		c.Manifest = path.Base(p.Techs[0].Files[0])
	}
	return c
}

// has reports whether the technology was detected, optionally only at the root.
func (p Profile) has(id string, rootOnly bool) bool {
	t, ok := p.Get(id)
	return ok && (!rootOnly || t.AtRoot())
}

func isFrontendPackage(manifest string) bool {
	b, err := os.ReadFile(manifest)
	if err != nil {
		return false
	}
	s := strings.ToLower(string(b))
	for _, w := range []string{`"react"`, `"vue"`, `"next"`, `"nuxt"`, `"vite"`, `"svelte"`, `"@angular/core"`, `"react-dom"`} {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// Surface is how a tester reaches what the repository builds: "web" (screens in a browser),
// "mobile" (an app installed on a device), "api" (endpoints only) or "" (nothing to test by hand,
// e.g. infrastructure code). A repository with no detected
// stack is assumed to be "web".
func (p Profile) Surface() string {
	switch {
	case len(p.Techs) == 0, p.Has("react"):
		return "web"
	case p.Has("flutter"), p.Has("android"):
		return "mobile"
	case p.Has("go"), p.Has("java"), p.Has("kotlin"), p.Has("dart"), p.Has("postgresql"), p.Has("openapi"), p.Has("spicedb"), p.Has("keycloak"):
		return "api"
	}
	for _, t := range p.Techs {
		if t.Category == Frontend || t.Category == Backend {
			return "web"
		}
	}
	return ""
}

// firstJVMBuild names the outermost Gradle or Maven build file, for display.
func firstJVMBuild(p Profile) string {
	if len(p.jvmBuilds) == 0 {
		return ""
	}
	return p.jvmBuilds[0].file
}
