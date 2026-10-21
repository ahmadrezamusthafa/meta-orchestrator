// Package stack detects the technologies a repository is built with — from its files only, never
// by running anything — and turns them into what the orchestrator needs: a role for the project
// wizard, tags for the UI and a short brief that tells agents how to verify a change in each
// stack, which conventions to keep and which commands they must never run.
//
// Adding a technology is two table entries: a rule in rules (how to recognise it) and, when agents
// need to know something about it, an entry in guides (brief.go).
package stack

import (
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Category groups technologies the way a team describes its stack.
type Category string

const (
	Backend       Category = "backend"
	Frontend      Category = "frontend"
	Identity      Category = "identity"
	Cloud         Category = "cloud"
	IaC           Category = "iac"
	Delivery      Category = "delivery"
	Observability Category = "observability"
	API           Category = "api"
	Mobile        Category = "mobile"
)

// Tech is one technology found in a repository.
type Tech struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Category Category `json:"category"`
	// Dirs are the repository-relative folders where it was found ("." is the root), shallowest first.
	Dirs []string `json:"dirs"`
	// Files are the files that revealed it (at most maxEvidence), repository-relative.
	Files []string `json:"files"`
}

// AtRoot reports whether the technology was found in the repository's root folder.
func (t Tech) AtRoot() bool { return len(t.Dirs) > 0 && t.Dirs[0] == "." }

// Profile is the detected stack of one repository.
type Profile struct {
	Techs []Tech `json:"techs"`
	// Truncated is set when the scan hit its file limit, so a technology may be missing.
	Truncated bool `json:"truncated,omitempty"`
	// Hosts are the hosts of the repository's git remotes, e.g. "github.com".
	Hosts []string `json:"hosts,omitempty"`
	// Trackers are the project-management tools the repository links to, e.g. "clickup" (see
	// trackers.go).
	Trackers []string `json:"trackers,omitempty"`

	nodePkgs  map[string]nodePkg // folder → package.json facts
	lints     map[string]bool    // folders holding a golangci-lint config
	jvmBuilds []jvmBuild         // Gradle/Maven build roots, shallowest first
	dartPkgs  []dartPkg          // pubspec.yaml packages, shallowest first
	luaDirs   []string           // folders holding Lua files, shallowest first
	others    []Package          // Ruby and Python manifests (packages.go), shallowest first
}

// Get returns the technology with id, if it was detected.
func (p Profile) Get(id string) (Tech, bool) {
	for _, t := range p.Techs {
		if t.ID == id {
			return t, true
		}
	}
	return Tech{}, false
}

// Has reports whether the technology with id was detected.
func (p Profile) Has(id string) bool {
	_, ok := p.Get(id)
	return ok
}

// Names lists the detected technologies' display names in stack order.
func (p Profile) Names() []string {
	out := make([]string, 0, len(p.Techs))
	for _, t := range p.Techs {
		out = append(out, t.Name)
	}
	return out
}

// signal is one way to recognise a technology: a file whose base name matches name (a path.Match
// glob), optionally inside a folder named under (any depth), optionally containing one of
// contains. Content is matched with a leading newline so "\nopenapi:" finds a top-level key.
type signal struct {
	name     string
	under    string
	contains []string
}

type rule struct {
	id, name string
	cat      Category
	signals  []signal
}

func file(name string, contains ...string) signal { return signal{name: name, contains: contains} }

func in(under, name string, contains ...string) signal {
	return signal{name: name, under: under, contains: contains}
}

var composeFiles = []string{"docker-compose*.yml", "docker-compose*.yaml", "compose.yml", "compose.yaml"}

func composeContains(words ...string) []signal {
	out := make([]signal, 0, len(composeFiles))
	for _, f := range composeFiles {
		out = append(out, file(f, words...))
	}
	return out
}

func join(groups ...[]signal) []signal {
	var out []signal
	for _, g := range groups {
		out = append(out, g...)
	}
	return out
}

// JVM build files, read for plugins and dependencies.
var jvmBuildFiles = []string{"pom.xml", "build.gradle", "build.gradle.kts"}

func jvmContains(words ...string) []signal {
	out := make([]signal, 0, len(jvmBuildFiles))
	for _, f := range jvmBuildFiles {
		out = append(out, file(f, words...))
	}
	return out
}

// rules are evaluated in this order, which is also the order technologies are reported in.
var rules = []rule{
	// Backend
	{id: "go", name: "Go", cat: Backend, signals: []signal{file("go.mod")}},
	{id: "postgresql", name: "PostgreSQL", cat: Backend, signals: join([]signal{
		file("go.mod", "github.com/jackc/pgx", "github.com/lib/pq", "gorm.io/driver/postgres", "github.com/uptrace/bun/dialect/pgdialect"),
		file("package.json", `"pg":`, `"postgres":`, `"pg-promise":`),
		file("sqlc.yaml", "postgresql"), file("sqlc.yml", "postgresql"), file("sqlc.json", "postgresql"),
		file("schema.prisma", `"postgresql"`),
		file("atlas.hcl", "postgres"),
		file("*.tf", "azurerm_postgresql"),
		file("pubspec.yaml", "\n  postgres:"),
	}, jvmContains("org.postgresql"), composeContains("postgres"))},
	{id: "java", name: "Java", cat: Backend, signals: join([]signal{file("*.java"), file("pom.xml", "<artifactId>maven-compiler-plugin", "<java.version>", "<maven.compiler")},
		jvmContains("id 'java'", `id("java")`, "id 'java-library'", "`java-library`", "apply plugin: 'java'", "id 'application'", "id(\"application\")"))},
	{id: "kotlin", name: "Kotlin", cat: Backend, signals: join([]signal{file("*.kt"), file("libs.versions.toml", "org.jetbrains.kotlin")},
		jvmContains("org.jetbrains.kotlin", `kotlin("`, "kotlin-android", "kotlin-maven-plugin", "kotlin-stdlib"))},

	// Frontend
	{id: "react", name: "React", cat: Frontend, signals: []signal{file("package.json", `"react":`)}},
	{id: "typescript", name: "TypeScript", cat: Frontend, signals: []signal{file("tsconfig.json"), file("package.json", `"typescript":`)}},

	// Mobile
	{id: "flutter", name: "Flutter", cat: Mobile, signals: []signal{file("pubspec.yaml", "sdk: flutter")}},
	{id: "dart", name: "Dart", cat: Mobile, signals: []signal{file("pubspec.yaml")}},
	{id: "android", name: "Android", cat: Mobile, signals: join([]signal{file("AndroidManifest.xml"),
		file("libs.versions.toml", "com.android.application", "com.android.library")},
		jvmContains("com.android.application", "com.android.library", "\nandroid {"))},

	// Identity
	{id: "keycloak", name: "Keycloak (OIDC)", cat: Identity, signals: join([]signal{
		file("package.json", "keycloak-js", "@react-keycloak", "oidc-client-ts", "react-oidc-context"),
		file("go.mod", "github.com/coreos/go-oidc", "github.com/Nerzal/gocloak", "github.com/zitadel/oidc"),
		file("pubspec.yaml", "flutter_appauth", "openid_client", "keycloak"),
		file("*realm*.json", `"realm"`),
		file("pom.xml", "org.keycloak", "spring-boot-starter-oauth2"),
		file("build.gradle", "org.keycloak", "spring-boot-starter-oauth2"), file("build.gradle.kts", "org.keycloak", "spring-boot-starter-oauth2"),
		file("*.tf", "keycloak_"),
	}, composeContains("keycloak"))},
	{id: "spicedb", name: "SpiceDB", cat: Identity, signals: join([]signal{
		file("*.zed"),
		file("go.mod", "github.com/authzed/"),
		file("pom.xml", "com.authzed"), file("build.gradle", "com.authzed"), file("build.gradle.kts", "com.authzed"),
		file("package.json", "@authzed/"),
	}, composeContains("spicedb"))},

	// Cloud
	{id: "azure-container-apps", name: "Azure Container Apps", cat: Cloud, signals: []signal{
		file("*.tf", "azurerm_container_app"),
		file("*.bicep", "Microsoft.App/containerApps"),
		file("terragrunt.hcl", "container_app", "container-app", "containerapp"),
		in(".github/workflows", "*.y*ml", "container-apps-deploy-action", "az containerapp"),
	}},
	{id: "cloudflare", name: "Cloudflare", cat: Cloud, signals: []signal{
		file("wrangler.toml"), file("wrangler.json"), file("wrangler.jsonc"),
		file("*.tf", "cloudflare/cloudflare", "cloudflare_"),
		file("package.json", `"wrangler":`, "@cloudflare/"),
		in(".github/workflows", "*.y*ml", "cloudflare/wrangler-action"),
	}},

	// Infrastructure as code
	{id: "opentofu", name: "OpenTofu", cat: IaC, signals: []signal{file("*.tf"), file("*.tofu"), file(".terraform.lock.hcl")}},
	{id: "terragrunt", name: "Terragrunt", cat: IaC, signals: []signal{
		file("terragrunt.hcl"), file("root.hcl", "remote_state", "include", "terraform {"),
	}},

	// Delivery
	{id: "github-actions", name: "GitHub Actions", cat: Delivery, signals: []signal{in(".github/workflows", "*.yml"), in(".github/workflows", "*.yaml")}},
	{id: "docker", name: "Containers (Docker)", cat: Delivery, signals: join([]signal{
		file("Dockerfile"), file("Dockerfile.*"), file("*.Dockerfile"), file("Containerfile"),
	}, composeContains())},
	{id: "playwright", name: "Playwright", cat: Delivery, signals: []signal{file("playwright.config.*"), file("package.json", "@playwright/test")}},

	// Observability
	{id: "opentelemetry", name: "OpenTelemetry", cat: Observability, signals: []signal{
		file("go.mod", "go.opentelemetry.io/otel"),
		file("pubspec.yaml", "opentelemetry"),
		file("pom.xml", "io.opentelemetry"), file("build.gradle", "io.opentelemetry"), file("build.gradle.kts", "io.opentelemetry"),
		file("package.json", "@opentelemetry/"),
		file("*otel*.yaml"), file("*otel*.yml"),
	}},
	{id: "grafana", name: "Grafana", cat: Observability, signals: []signal{
		in("dashboards", "*.json", `"panels"`),
		in("grafana", "*"),
		file("*.tf", "grafana/grafana", "grafana_dashboard"),
		file("*.jsonnet", "grafonnet"), file("*.libsonnet", "grafonnet"),
	}},

	// API
	{id: "nginx", name: "Nginx", cat: API, signals: []signal{
		file("nginx*.conf"), in("nginx", "*.conf"), in("conf.d", "*.conf"),
		file("*.conf", "proxy_pass", "server_name", "\nupstream "),
		file("*.conf.template", "proxy_pass", "server_name"),
	}},
	{id: "openresty", name: "OpenResty (Lua)", cat: API, signals: join([]signal{
		file("*.conf", "_by_lua", "lua_shared_dict", "lua_package_path"),
		file("*.lua", "ngx.", "\"resty.", "'resty."),
		file("Dockerfile", "openresty"), file("Dockerfile.*", "openresty"),
	}, composeContains("openresty"))},
	{id: "openapi", name: "OpenAPI", cat: API, signals: []signal{
		file("openapi*.yaml"), file("openapi*.yml"), file("openapi*.json"),
		file("swagger*.yaml"), file("swagger*.yml"), file("swagger*.json"),
		file("*.yaml", "\nopenapi:"), file("*.yml", "\nopenapi:"),
	}},
}

// skipDirs are never descended into: dependencies, build output and tool caches.
var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "out": true, "bin": true,
	"coverage": true, "tmp": true, "target": true, ".next": true, ".nuxt": true, ".cache": true, ".turbo": true,
	".terraform": true, ".terragrunt-cache": true, "__pycache__": true, ".venv": true, "playwright-report": true,
	"test-results": true, "Pods": true, "DerivedData": true,
}

// Options bound a scan.
type Options struct {
	MaxDepth int // folder levels below the root to descend (0 = root files only)
	MaxFiles int // files to look at before giving up (the profile is then Truncated)
}

// Deep is for agent briefs and registered repositories; Quick is for browsing many folders.
var (
	Deep  = Options{MaxDepth: 4, MaxFiles: 5000}
	Quick = Options{MaxDepth: 2, MaxFiles: 400}
)

const (
	maxRead     = 64 << 10 // bytes of a file read for content signals
	maxEvidence = 3
	maxDirs     = 6
)

// Detect scans the repository at root and reports the technologies it uses.
func Detect(root string, opt Options) Profile {
	files, truncated := listFiles(root, opt)
	p := Profile{Truncated: truncated, nodePkgs: map[string]nodePkg{}, lints: map[string]bool{}}
	reader := &contentReader{root: root, cache: map[string]string{}}
	files = dropFlutterScaffolding(files, reader)

	for _, r := range rules {
		var t *Tech
		for _, rel := range files {
			if !r.matches(rel, reader) {
				continue
			}
			if t == nil {
				t = &Tech{ID: r.id, Name: r.name, Category: r.cat}
			}
			addUnique(&t.Dirs, dirOf(rel), maxDirs)
			if len(t.Files) < maxEvidence {
				t.Files = append(t.Files, rel)
			}
		}
		if t != nil {
			sort.SliceStable(t.Dirs, func(i, j int) bool { return depth(t.Dirs[i]) < depth(t.Dirs[j]) })
			p.Techs = append(p.Techs, *t)
		}
	}

	for _, rel := range files {
		switch base := path.Base(rel); {
		case base == "package.json" && len(p.nodePkgs) < maxDirs:
			p.nodePkgs[dirOf(rel)] = readNodePkg(root, rel, files)
		case strings.HasPrefix(base, ".golangci."):
			p.lints[dirOf(rel)] = true
		case strings.HasSuffix(base, ".lua"):
			addUnique(&p.luaDirs, dirOf(rel), maxDirs)
		case base == "pubspec.yaml" && len(p.dartPkgs) < maxDirs:
			p.dartPkgs = append(p.dartPkgs, readDartPkg(root, rel, reader))
		default:
			p.addOtherManifest(rel)
		}
	}
	p.jvmBuilds = jvmBuildRoots(files)
	p.Hosts = gitHosts(root)
	p.Trackers = trackerRefs(root, files, reader)
	return p
}

// flutterPlatforms are the native host projects `flutter create` generates beside lib/. Their
// Gradle, Kotlin and Swift files belong to the Flutter app, not to separate Android or iOS apps.
var flutterPlatforms = []string{"android", "ios", "macos", "linux", "windows", "web"}

func dropFlutterScaffolding(files []string, reader *contentReader) []string {
	var prefixes []string
	for _, rel := range files {
		if path.Base(rel) == "pubspec.yaml" && strings.Contains(reader.read(rel), "sdk: flutter") {
			for _, pl := range flutterPlatforms {
				prefixes = append(prefixes, joinRel(dirOf(rel), pl)+"/")
			}
		}
	}
	if len(prefixes) == 0 {
		return files
	}
	out := files[:0:0]
next:
	for _, rel := range files {
		for _, pre := range prefixes {
			if strings.HasPrefix(rel, pre) {
				continue next
			}
		}
		out = append(out, rel)
	}
	return out
}

// jvmBuild is a Gradle or Maven build to run from its root folder.
type jvmBuild struct {
	dir     string
	file    string // the build file that marked it
	gradle  bool
	wrapper bool // ./gradlew or ./mvnw is checked in
}

// jvmBuildRoots keeps the outermost build of each tree: a multi-module build runs from its root,
// never once per module.
func jvmBuildRoots(files []string) []jvmBuild {
	present := map[string]bool{}
	for _, f := range files {
		present[f] = true
	}
	var builds []jvmBuild
	seen := map[string]bool{}
	for _, rel := range files {
		base, dir := path.Base(rel), dirOf(rel)
		gradle := strings.HasPrefix(base, "build.gradle") || strings.HasPrefix(base, "settings.gradle")
		if (!gradle && base != "pom.xml") || seen[dir] {
			continue
		}
		seen[dir] = true
		wrapper := present[joinRel(dir, "mvnw")]
		if gradle {
			wrapper = present[joinRel(dir, "gradlew")]
		}
		builds = append(builds, jvmBuild{dir: dir, file: rel, gradle: gradle, wrapper: wrapper})
	}
	sort.SliceStable(builds, func(i, j int) bool { return depth(builds[i].dir) < depth(builds[j].dir) })
	var roots []jvmBuild
	for _, b := range builds {
		nested := false
		for _, r := range roots {
			nested = nested || r.dir == "." || strings.HasPrefix(b.dir+"/", r.dir+"/")
		}
		if !nested && len(roots) < maxDirs {
			roots = append(roots, b)
		}
	}
	return roots
}

// dartPkg is what the brief needs from a pubspec.yaml.
type dartPkg struct {
	dir         string
	flutter     bool
	tests       bool // the package has a test/ folder
	buildRunner bool // code generation (json_serializable, freezed, …) through build_runner
}

func readDartPkg(root, rel string, reader *contentReader) dartPkg {
	text := reader.read(rel)
	dir := dirOf(rel)
	st, err := os.Stat(filepath.Join(root, filepath.FromSlash(joinRel(dir, "test"))))
	return dartPkg{dir: dir, flutter: strings.Contains(text, "sdk: flutter"), tests: err == nil && st.IsDir(),
		buildRunner: strings.Contains(text, "build_runner")}
}

func (r rule) matches(rel string, reader *contentReader) bool {
	base := path.Base(rel)
	for _, s := range r.signals {
		if ok, _ := path.Match(s.name, base); !ok {
			continue
		}
		if s.under != "" && !strings.Contains("/"+dirOf(rel)+"/", "/"+s.under+"/") {
			continue
		}
		if len(s.contains) == 0 {
			return true
		}
		text := reader.read(rel)
		for _, c := range s.contains {
			if strings.Contains(text, c) {
				return true
			}
		}
	}
	return false
}

// listFiles walks root breadth-first so the shallowest files are seen first when the cap is hit.
// Hidden folders are skipped except .github, where workflows live.
func listFiles(root string, opt Options) ([]string, bool) {
	type dir struct {
		rel   string
		depth int
	}
	var files []string
	queue := []dir{{rel: ".", depth: 0}}
	for visited := 0; len(queue) > 0; visited++ {
		if visited >= opt.MaxFiles/2 { // folder-heavy trees (home, Library) end the scan too
			return files, true
		}
		d := queue[0]
		queue = queue[1:]
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(d.rel)))
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			rel := name
			if d.rel != "." {
				rel = d.rel + "/" + name
			}
			if e.IsDir() {
				hidden := strings.HasPrefix(name, ".") && name != ".github"
				if d.depth < opt.MaxDepth && !skipDirs[name] && !hidden {
					queue = append(queue, dir{rel: rel, depth: d.depth + 1})
				}
				continue
			}
			if !e.Type().IsRegular() {
				continue
			}
			if len(files) >= opt.MaxFiles {
				return files, true
			}
			files = append(files, rel)
		}
	}
	return files, false
}

type contentReader struct {
	root  string
	cache map[string]string
}

// read returns the head of a file, with a leading newline for line-start matches.
func (c *contentReader) read(rel string) string {
	if s, ok := c.cache[rel]; ok {
		return s
	}
	s := "\n"
	if f, err := os.Open(filepath.Join(c.root, filepath.FromSlash(rel))); err == nil {
		b, _ := io.ReadAll(io.LimitReader(f, maxRead))
		f.Close()
		s += string(b)
	}
	c.cache[rel] = s
	return s
}

// nodePkg is what the brief needs from a package.json: how to run its scripts.
type nodePkg struct {
	manager string // npm, pnpm, yarn or bun
	scripts map[string]bool
}

// readNodePkg reads scripts and picks the package manager from the nearest lockfile (the package's
// folder, then the root of a workspace).
func readNodePkg(root, rel string, files []string) nodePkg {
	pkg := nodePkg{manager: "npm", scripts: map[string]bool{}}
	if b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
		var manifest struct {
			Scripts        map[string]string `json:"scripts"`
			PackageManager string            `json:"packageManager"`
		}
		if json.Unmarshal(b, &manifest) == nil {
			for s := range manifest.Scripts {
				pkg.scripts[s] = true
			}
			if pm := strings.SplitN(manifest.PackageManager, "@", 2)[0]; pm != "" {
				pkg.manager = pm
				return pkg
			}
		}
	}
	present := map[string]bool{}
	for _, f := range files {
		present[f] = true
	}
	lockfiles := []struct{ file, manager string }{
		{"pnpm-lock.yaml", "pnpm"}, {"yarn.lock", "yarn"}, {"bun.lockb", "bun"}, {"bun.lock", "bun"},
	}
	for _, dir := range []string{dirOf(rel), "."} {
		for _, l := range lockfiles {
			if present[joinRel(dir, l.file)] {
				pkg.manager = l.manager
				return pkg
			}
		}
	}
	return pkg
}

func dirOf(rel string) string { return path.Dir(rel) }

func joinRel(dir, name string) string {
	if dir == "." {
		return name
	}
	return dir + "/" + name
}

func depth(rel string) int {
	if rel == "." {
		return 0
	}
	return strings.Count(rel, "/") + 1
}

func addUnique(list *[]string, v string, max int) {
	for _, x := range *list {
		if x == v {
			return
		}
	}
	if len(*list) < max {
		*list = append(*list, v)
	}
}

// cache keeps deep profiles for a short while: briefs are built on every stage turn and chat
// message, while a repository's stack rarely changes within minutes.
var cache = struct {
	sync.Mutex
	entries map[string]cached
}{entries: map[string]cached{}}

type cached struct {
	profile Profile
	at      time.Time
}

const cacheTTL = 2 * time.Minute

// For returns the deep profile of the repository at root, cached for a couple of minutes. A path
// that is not a folder has an empty profile.
func For(root string) Profile {
	if root == "" {
		return Profile{}
	}
	cache.Lock()
	if c, ok := cache.entries[root]; ok && time.Since(c.at) < cacheTTL {
		cache.Unlock()
		return c.profile
	}
	cache.Unlock()
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return Profile{}
	}
	p := Detect(root, Deep)
	cache.Lock()
	cache.entries[root] = cached{profile: p, at: time.Now()}
	cache.Unlock()
	return p
}

// sortedKeys orders package folders shallowest first, then by name, for stable briefs.
func sortedKeys(m map[string]nodePkg) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if di, dj := depth(keys[i]), depth(keys[j]); di != dj {
			return di < dj
		}
		return keys[i] < keys[j]
	})
	return keys
}
