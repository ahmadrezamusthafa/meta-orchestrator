package deps

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// write creates files under root; parent folders are created as needed.
func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// age moves a file's modification time into the past (negative d) or future.
func age(t *testing.T, p string, d time.Duration) {
	t.Helper()
	at := time.Now().Add(d)
	if err := os.Chtimes(p, at, at); err != nil {
		t.Fatal(err)
	}
}

// allFound finds every binary; noneFound finds none.
func allFound(name string) (string, error) { return "/usr/local/bin/" + name, nil }
func noneFound(string) (string, error)     { return "", errors.New("not found") }

func scanOne(t *testing.T, root, eco string, opt Options) Ecosystem {
	t.Helper()
	if opt.LookPath == nil {
		opt.LookPath = allFound
	}
	if opt.GoModCache == "" {
		opt.GoModCache = t.TempDir()
	}
	e, ok := Find(Scan(root, opt), eco)
	if !ok {
		t.Fatalf("no %s ecosystem in %v", eco, Scan(root, opt))
	}
	return e
}

func dep(t *testing.T, e Ecosystem, name string) Dependency {
	t.Helper()
	for _, d := range e.Dependencies {
		if d.Name == name {
			return d
		}
	}
	t.Fatalf("no dependency %s in %+v", name, e.Dependencies)
	return Dependency{}
}

const npmLock = `{"lockfileVersion": 3, "packages": {
 "": {"name": "web"},
 "node_modules/react": {"version": "18.3.1"},
 "node_modules/vitest": {"version": "1.6.0"}}}`

func TestNodeNpmInstalledStateAndDrift(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"package.json":      `{"dependencies": {"react": "^18.2.0", "left-pad": "1.3.0"}, "devDependencies": {"vitest": "^1.0.0"}}`,
		"package-lock.json": npmLock,
	})
	e := scanOne(t, root, "node", Options{})
	if e.Manager != "npm" || e.Lockfile != "package-lock.json" || e.Manifest != "package.json" {
		t.Fatalf("detection: %+v", e)
	}
	if !reflect.DeepEqual(e.Install.Command, []string{"npm", "ci"}) || !e.Install.Auto || e.MissingTool != "" {
		t.Fatalf("install: %+v missing=%q", e.Install, e.MissingTool)
	}
	if e.Installed || e.Drift != "" {
		t.Fatalf("without node_modules nothing is installed and there is no drift: %+v", e)
	}
	if r := dep(t, e, "react"); !r.Key || r.Dev || r.Wanted != "^18.2.0" || r.Installed != "18.3.1" {
		t.Fatalf("react (installed falls back to the lockfile): %+v", r)
	}
	if v := dep(t, e, "vitest"); !v.Dev || !v.Key {
		t.Fatalf("vitest: %+v", v)
	}
	if lp := dep(t, e, "left-pad"); lp.Key {
		t.Fatalf("left-pad is not a key dependency")
	}
	if e.Dependencies[len(e.Dependencies)-1].Name != "vitest" {
		t.Fatalf("dev dependencies sort last: %+v", e.Dependencies)
	}

	write(t, root, map[string]string{
		"node_modules/.package-lock.json":    "{}",
		"node_modules/react/package.json":    `{"version": "18.3.1"}`,
		"node_modules/vitest/package.json":   `{"version": "1.6.0"}`,
		"node_modules/left-pad/package.json": `{"version": "1.3.0"}`,
	})
	if e := scanOne(t, root, "node", Options{}); !e.Installed || e.Drift != "" {
		t.Fatalf("fresh node_modules is installed: %+v", e)
	}

	age(t, filepath.Join(root, "package-lock.json"), time.Hour)
	if e := scanOne(t, root, "node", Options{}); e.Installed || !strings.Contains(e.Drift, "older than package-lock.json") ||
		!strings.Contains(e.Drift, "npm ci") {
		t.Fatalf("a lockfile newer than node_modules is drift: %+v", e)
	}
	age(t, filepath.Join(root, "package-lock.json"), -time.Hour)

	write(t, root, map[string]string{"node_modules/react/package.json": `{"version": "18.2.0"}`})
	e = scanOne(t, root, "node", Options{})
	if e.Installed || !strings.Contains(e.Drift, "react: node_modules has 18.2.0, the lockfile pins 18.3.1") {
		t.Fatalf("a version that differs from the lockfile is drift: %+v", e)
	}
	if r := dep(t, e, "react"); r.Installed != "18.2.0" {
		t.Fatalf("installed reports node_modules: %+v", r)
	}
}

func TestNodeManagersAndLockfiles(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  []string
		mgr   string
		lock  string
		note  string
		react string
	}{
		{name: "pnpm", files: map[string]string{
			"package.json":   `{"dependencies": {"react": "^18.2.0"}}`,
			"pnpm-lock.yaml": "lockfileVersion: '9.0'\nimporters:\n  .:\n    dependencies:\n      react:\n        specifier: ^18.2.0\n        version: 18.3.1(typescript@5.4.0)\n",
		}, want: []string{"pnpm", "install", "--frozen-lockfile"}, mgr: "pnpm", lock: "pnpm-lock.yaml", react: "18.3.1"},
		{name: "yarn classic", files: map[string]string{
			"package.json": `{"dependencies": {"react": "^18.2.0"}}`,
			"yarn.lock":    "# yarn lockfile v1\n\n\nreact@^18.0.0, react@^18.2.0:\n  version \"18.3.1\"\n  resolved \"https://registry.yarnpkg.com/react/-/react-18.3.1.tgz\"\n  dependencies:\n    loose-envify \"^1.1.0\"\n",
		}, want: []string{"yarn", "install", "--frozen-lockfile"}, mgr: "yarn", lock: "yarn.lock", react: "18.3.1"},
		{name: "yarn berry via .yarnrc.yml", files: map[string]string{
			"package.json": `{"dependencies": {"react": "^18.2.0"}}`,
			".yarnrc.yml":  "nodeLinker: node-modules\n",
			"yarn.lock":    "__metadata:\n  version: 8\n\n\"react@npm:^18.2.0\":\n  version: 18.3.1\n  resolution: \"react@npm:18.3.1\"\n",
		}, want: []string{"yarn", "install", "--immutable"}, mgr: "yarn", lock: "yarn.lock", react: "18.3.1"},
		{name: "yarn berry via packageManager", files: map[string]string{
			"package.json": `{"packageManager": "yarn@4.1.0", "dependencies": {"react": "^18.2.0"}}`,
			"yarn.lock":    "",
		}, want: []string{"yarn", "install", "--immutable"}, mgr: "yarn", lock: "yarn.lock"},
		{name: "bun", files: map[string]string{
			"package.json": `{"dependencies": {"react": "^18.2.0"}}`,
			"bun.lock":     `{"lockfileVersion": 1, "packages": {"react": ["react@18.3.1", "", {}, "sha512-x"]}}`,
		}, want: []string{"bun", "install", "--frozen-lockfile"}, mgr: "bun", lock: "bun.lock", react: "18.3.1"},
		{name: "npm shrinkwrap", files: map[string]string{
			"package.json":        `{"dependencies": {"react": "^18.2.0"}}`,
			"npm-shrinkwrap.json": npmLock,
		}, want: []string{"npm", "ci"}, mgr: "npm", lock: "npm-shrinkwrap.json", react: "18.3.1"},
		{name: "no lockfile", files: map[string]string{
			"package.json": `{"dependencies": {"react": "^18.2.0"}}`,
		}, want: []string{"npm", "install"}, mgr: "npm", note: "No lockfile — versions may drift."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, c.files)
			e := scanOne(t, root, "node", Options{})
			if e.Manager != c.mgr || e.Lockfile != c.lock || !reflect.DeepEqual(e.Install.Command, c.want) || !e.Install.Auto {
				t.Fatalf("got manager=%s lock=%s install=%+v", e.Manager, e.Lockfile, e.Install)
			}
			if c.note != "" && !strings.Contains(e.Install.Note, c.note) {
				t.Fatalf("note %q", e.Install.Note)
			}
			if r := dep(t, e, "react"); r.Installed != c.react {
				t.Fatalf("react locked at %q, want %q", r.Installed, c.react)
			}
		})
	}
}

func TestMissingManagerNamesToolchainTool(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{"package.json": `{}`, "pnpm-lock.yaml": "lockfileVersion: '9.0'\n"})
	e := scanOne(t, root, "node", Options{LookPath: noneFound})
	if e.MissingTool != "pnpm" || !e.Missing || e.Runnable() {
		t.Fatalf("missing pnpm: %+v", e)
	}
	write(t, root, map[string]string{"package.json": `{"packageManager": "npm@10.0.0"}`})
	if e := scanOne(t, root, "node", Options{LookPath: noneFound}); e.MissingTool != "node" {
		t.Fatalf("npm comes with node: %+v", e)
	}
}

func TestGoModulesAgainstModuleCache(t *testing.T) {
	root, cache := t.TempDir(), t.TempDir()
	write(t, root, map[string]string{
		"go.mod": "module example.com/svc\n\ngo 1.22\n\nrequire github.com/gin-gonic/gin v1.10.0\n\nrequire (\n\tgithub.com/BurntSushi/toml v1.3.2 // indirect\n\tgithub.com/jackc/pgx/v5 v5.5.0\n)\n",
		"go.sum": "",
	})
	if err := os.MkdirAll(filepath.Join(cache, "github.com", "!burnt!sushi", "toml@v1.3.2"), 0o755); err != nil {
		t.Fatal(err)
	}
	e := scanOne(t, root, "go", Options{GoModCache: cache})
	if !reflect.DeepEqual(e.Install.Command, []string{"go", "mod", "download"}) || e.Lockfile != "go.sum" || e.Manager != "go" {
		t.Fatalf("go: %+v", e)
	}
	if len(e.Dependencies) != 3 || e.Installed || !strings.Contains(e.Drift, "2 of 3 required modules") {
		t.Fatalf("partial cache is drift: %+v", e)
	}
	if d := dep(t, e, "github.com/BurntSushi/toml"); d.Installed != "v1.3.2" {
		t.Fatalf("case-escaped module cache path: %+v", d)
	}
	if !dep(t, e, "github.com/gin-gonic/gin").Key || !dep(t, e, "github.com/jackc/pgx/v5").Key {
		t.Fatal("gin and pgx are key")
	}
	for _, m := range []string{"github.com/gin-gonic/gin@v1.10.0", "github.com/jackc/pgx/v5@v5.5.0"} {
		_ = os.MkdirAll(filepath.Join(cache, filepath.FromSlash(m)), 0o755)
	}
	if e := scanOne(t, root, "go", Options{GoModCache: cache}); !e.Installed || e.Drift != "" {
		t.Fatalf("full cache is installed: %+v", e)
	}
}

func TestFlutterAndDart(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"pubspec.yaml": "name: app\ndependencies:\n  flutter:\n    sdk: flutter\n  flutter_bloc: ^8.1.0\n  local_pkg:\n    path: ../local\ndev_dependencies:\n  flutter_test:\n    sdk: flutter\n  mocktail: ^1.0.0\n",
		"pubspec.lock": "packages:\n  flutter_bloc:\n    dependency: \"direct main\"\n    version: \"8.1.5\"\n  mocktail:\n    version: \"1.0.3\"\n",
	})
	e := scanOne(t, root, "flutter", Options{})
	if !reflect.DeepEqual(e.Install.Command, []string{"flutter", "pub", "get"}) || e.Lockfile != "pubspec.lock" {
		t.Fatalf("flutter: %+v", e)
	}
	if len(e.Dependencies) != 3 {
		t.Fatalf("sdk dependencies are skipped: %+v", e.Dependencies)
	}
	if b := dep(t, e, "flutter_bloc"); !b.Key || b.Installed != "8.1.5" || b.Wanted != "^8.1.0" {
		t.Fatalf("flutter_bloc: %+v", b)
	}
	if m := dep(t, e, "mocktail"); !m.Dev || m.Installed != "1.0.3" {
		t.Fatalf("mocktail: %+v", m)
	}
	if p := dep(t, e, "local_pkg"); p.Wanted != "path" {
		t.Fatalf("path dependency: %+v", p)
	}
	if e.Installed {
		t.Fatal("no .dart_tool yet")
	}
	write(t, root, map[string]string{".dart_tool/package_config.json": "{}"})
	if e := scanOne(t, root, "flutter", Options{}); !e.Installed {
		t.Fatalf("package_config newer than the lock is installed: %+v", e)
	}
	age(t, filepath.Join(root, ".dart_tool", "package_config.json"), -time.Hour)
	if e := scanOne(t, root, "flutter", Options{LookPath: noneFound}); e.Installed || !strings.Contains(e.Drift, "older than pubspec.lock") || e.MissingTool != "flutter" {
		t.Fatalf("stale package_config is drift: %+v", e)
	}

	dart := t.TempDir()
	write(t, dart, map[string]string{"pubspec.yaml": "name: cli\ndependencies:\n  args: ^2.4.0\n"})
	if e := scanOne(t, dart, "dart", Options{}); !reflect.DeepEqual(e.Install.Command, []string{"dart", "pub", "get"}) ||
		!strings.Contains(e.Install.Note, "No lockfile") {
		t.Fatalf("dart: %+v", e)
	}
}

func TestRubyPythonJVM(t *testing.T) {
	root := t.TempDir()
	write(t, root, map[string]string{
		"Gemfile":          "source 'https://rubygems.org'\ngem 'rails', '~> 7.1'\ngroup :development, :test do\n  gem \"rspec-rails\"\nend\ngem 'puma'\n",
		"Gemfile.lock":     "GEM\n  remote: https://rubygems.org/\n  specs:\n    rails (7.1.3)\n      actionpack (= 7.1.3)\n    puma (6.4.2)\n",
		"requirements.txt": "# deps\nDjango==5.0.1\nrequests>=2.31 ; python_version > '3.8'\n-r other.txt\n",
		"gradlew":          "#!/bin/sh\n",
		"build.gradle.kts": "plugins { id(\"java\") }\ndependencies {\n  implementation(\"org.springframework.boot:spring-boot-starter-web:3.2.0\")\n  testImplementation(\"org.junit.jupiter:junit-jupiter:5.10.0\")\n}\n",
	})
	_ = os.Chmod(filepath.Join(root, "gradlew"), 0o755)

	rb := scanOne(t, root, "ruby", Options{LookPath: noneFound})
	if rb.Manager != "bundler" || !reflect.DeepEqual(rb.Install.Command, []string{"bundle", "install"}) || rb.Lockfile != "Gemfile.lock" ||
		!rb.Missing || rb.MissingTool != "" {
		t.Fatalf("ruby: %+v", rb)
	}
	if r := dep(t, rb, "rails"); r.Wanted != "~> 7.1" || r.Installed != "7.1.3" || !r.Key || r.Dev {
		t.Fatalf("rails: %+v", r)
	}
	if r := dep(t, rb, "rspec-rails"); !r.Dev {
		t.Fatalf("rspec-rails is in the test group: %+v", r)
	}
	if r := dep(t, rb, "puma"); r.Dev || r.Installed != "6.4.2" {
		t.Fatalf("puma after the group: %+v", r)
	}

	py := scanOne(t, root, "python", Options{})
	if py.Install.Auto || py.Manager != "pip" || !strings.Contains(py.Install.Note, "No virtual environment") {
		t.Fatalf("requirements.txt without a venv is copy-only: %+v", py)
	}
	if d := dep(t, py, "Django"); d.Wanted != "==5.0.1" || !d.Key {
		t.Fatalf("django: %+v", d)
	}
	if d := dep(t, py, "requests"); d.Wanted != ">=2.31" {
		t.Fatalf("environment markers are dropped: %+v", d)
	}
	write(t, root, map[string]string{".venv/bin/python": "#!/bin/sh\n"})
	if py := scanOne(t, root, "python", Options{}); !py.Runnable() || py.Install.Command[0] != ".venv/bin/python" {
		t.Fatalf("a project .venv makes it runnable: %+v", py)
	}

	jvm := scanOne(t, root, "jvm", Options{})
	if jvm.Manager != "gradle" || !reflect.DeepEqual(jvm.Install.Command, []string{"./gradlew", "dependencies"}) || !jvm.Runnable() ||
		jvm.Binary() != filepath.Join(root, "gradlew") {
		t.Fatalf("gradle wrapper: %+v bin=%s", jvm, jvm.Binary())
	}
	if d := dep(t, jvm, "org.junit.jupiter:junit-jupiter"); !d.Dev || !d.Key || d.Wanted != "5.10.0" {
		t.Fatalf("junit: %+v", d)
	}
	if jvm := scanOne(t, root, "jvm", Options{LookPath: noneFound}); jvm.MissingTool != "java" || jvm.Runnable() {
		t.Fatalf("the wrapper needs a JDK: %+v", jvm)
	}

	mvn := t.TempDir()
	write(t, mvn, map[string]string{"pom.xml": `<project><properties><spring.version>3.2.1</spring.version></properties>
<dependencies><dependency><groupId>org.springframework.boot</groupId><artifactId>spring-boot-starter</artifactId><version>${spring.version}</version></dependency>
<dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4.13.2</version><scope>test</scope></dependency></dependencies></project>`})
	m := scanOne(t, mvn, "jvm", Options{LookPath: noneFound})
	if m.Manager != "maven" || m.Install.Auto || m.MissingTool != "maven" || m.Install.Command[0] != "mvn" {
		t.Fatalf("maven without a wrapper is copy-only: %+v", m)
	}
	if d := dep(t, m, "org.springframework.boot:spring-boot-starter"); d.Wanted != "3.2.1" {
		t.Fatalf("property versions resolve: %+v", d)
	}

	uv := t.TempDir()
	write(t, uv, map[string]string{
		"pyproject.toml": "[project]\nname = \"svc\"\ndependencies = [\n  \"fastapi>=0.110\",\n  \"pydantic\",\n]\n",
		"uv.lock":        "version = 1\n\n[[package]]\nname = \"fastapi\"\nversion = \"0.111.0\"\n",
	})
	u := scanOne(t, uv, "python", Options{})
	if !reflect.DeepEqual(u.Install.Command, []string{"uv", "sync", "--frozen"}) || !u.Install.Auto || u.Lockfile != "uv.lock" {
		t.Fatalf("uv: %+v", u)
	}
	if d := dep(t, u, "fastapi"); d.Installed != "0.111.0" || d.Wanted != ">=0.110" {
		t.Fatalf("fastapi: %+v", d)
	}
}

// fakeBin writes an executable shell script named name into dir.
func fakeBin(t *testing.T, dir, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func npmRepo(t *testing.T, script string) (string, Ecosystem) {
	t.Helper()
	bin, root := t.TempDir(), t.TempDir()
	fakeBin(t, bin, "npm", script)
	t.Setenv("PATH", bin)
	write(t, root, map[string]string{"package.json": `{"dependencies": {"react": "^18.2.0"}}`, "package-lock.json": npmLock})
	e, _ := Find(Scan(root, Options{}), "node")
	if !e.Runnable() || e.Binary() != filepath.Join(bin, "npm") {
		t.Fatalf("fake npm should be found: %+v", e)
	}
	return root, e
}

func waitJob(t *testing.T, s *Service, id string) Job {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if j, _ := s.Get(id); j.Done() {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s did not finish", id)
	return Job{}
}

func TestServiceRunsInstallAndStreamsLog(t *testing.T) {
	root, e := npmRepo(t, "echo \"npm args: $*\"\necho \"CI=$CI\"\necho 'fetch https://user:s3cr3t@registry.example.com/react' 1>&2\n/bin/mkdir -p node_modules/react\n")
	s := NewService()
	var mu sync.Mutex
	var updates []Job
	s.OnUpdate = func(j Job) { mu.Lock(); updates = append(updates, j); mu.Unlock() }

	j, err := s.Start("repo:web", root, e)
	if err != nil || j.Package != "repo:web" || j.Action != "install" || j.State != StateQueued || !strings.HasPrefix(j.ID, "dep-") {
		t.Fatalf("start: %+v %v", j, err)
	}
	done := waitJob(t, s, j.ID)
	s.Wait()
	if done.State != StateSucceeded || done.Finished.IsZero() || !strings.Contains(done.Message, "npm ci finished") {
		t.Fatalf("job: %+v", done)
	}
	log := strings.Join(done.Log, "\n")
	for _, want := range []string{"$ npm ci", "npm args: ci", "CI=true", "https://***@registry.example.com/react"} {
		if !strings.Contains(log, want) {
			t.Fatalf("log lacks %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "s3cr3t") {
		t.Fatal("credentials in URLs are masked")
	}
	if _, err := os.Stat(filepath.Join(root, "node_modules", "react")); err != nil {
		t.Fatal("install runs in the repository folder")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(updates) < 3 || updates[0].State != StateQueued || updates[len(updates)-1].State != StateSucceeded {
		t.Fatalf("updates on every state change: %+v", updates)
	}
	if last, ok := s.Last("repo:web"); !ok || last.ID != j.ID {
		t.Fatalf("last job: %+v", last)
	}
}

func TestServiceOneInstallPerRepo(t *testing.T) {
	root, e := npmRepo(t, "/bin/sleep 1\n")
	s := NewService()
	j, err := s.Start("repo:web", root, e)
	if err != nil {
		t.Fatal(err)
	}
	busy, err := s.Start("repo:web", root, e)
	if !errors.Is(err, ErrBusy) || busy.ID != j.ID {
		t.Fatalf("second install for the same repo: %+v %v", busy, err)
	}
	other, err := s.Start("repo:api", root, e)
	if err != nil {
		t.Fatalf("another repo may install meanwhile: %v", err)
	}
	waitJob(t, s, j.ID)
	waitJob(t, s, other.ID)
	if _, err := s.Start("repo:web", root, e); err != nil {
		t.Fatalf("the repo is free again: %v", err)
	}
	s.Wait()
}

func TestServiceTimeoutKillsProcessGroup(t *testing.T) {
	root, e := npmRepo(t, "echo started\n/bin/sleep 30\n")
	s := NewService()
	s.Timeout = 300 * time.Millisecond
	start := time.Now()
	j, _ := s.Start("repo:web", root, e)
	done := waitJob(t, s, j.ID)
	if done.State != StateFailed || !strings.Contains(done.Message, "timed out") {
		t.Fatalf("timeout: %+v", done)
	}
	// The sleep child holds the output pipe: only killing the whole group ends the job quickly.
	if took := time.Since(start); took > 4*time.Second {
		t.Fatalf("the process group should be killed at once, took %s", took)
	}
	s.Wait()
}

func TestServiceFailureExitCode(t *testing.T) {
	root, e := npmRepo(t, "echo 'npm ERR! lockfile out of sync'\nexit 3\n")
	s := NewService()
	j, _ := s.Start("repo:web", root, e)
	done := waitJob(t, s, j.ID)
	if done.State != StateFailed || !strings.Contains(done.Message, "exited with code 3") {
		t.Fatalf("failure: %+v", done)
	}
	s.Wait()
}

func TestRedact(t *testing.T) {
	for in, leak := range map[string]string{
		"//registry.npmjs.org/:_authToken=npm_abcdefghijklmnop1234":    "abcdefghijklmnop1234",
		"Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload":           "eyJhbGciOiJIUzI1NiJ9",
		"cloning https://x-access-token:ghs_secretvalue123@github.com": "ghs_secretvalue123",
		"password=hunter2":                       "hunter2",
		"token ghp_0123456789abcdefghij appears": "ghp_0123456789abcdefghij",
	} {
		if out := Redact(in); strings.Contains(out, leak) {
			t.Fatalf("Redact(%q) = %q still holds the secret", in, out)
		}
	}
	if out := Redact("added 120 packages in 3s"); out != "added 120 packages in 3s" {
		t.Fatalf("ordinary output is unchanged: %q", out)
	}
}

func TestLatestOnlyRunsNpmViewOnceAndIsCached(t *testing.T) {
	bin, root := t.TempDir(), t.TempDir()
	calls := filepath.Join(t.TempDir(), "calls")
	fakeBin(t, bin, "npm", "echo \"$*\" >> "+calls+"\nif [ \"$2\" = react ]; then echo 19.1.0; else echo 2.0.0; fi\n")
	t.Setenv("PATH", bin)
	write(t, root, map[string]string{"package.json": `{"dependencies": {"react": "^18.2.0", "Bad Name": "1"}, "devDependencies": {"vitest": "^1.0.0"}}`})

	l := NewLatest()
	ecos := Scan(root, Options{})
	l.Fill(context.Background(), root, ecos, nil)
	e, _ := Find(ecos, "node")
	if dep(t, e, "react").Latest != "19.1.0" || dep(t, e, "vitest").Latest != "2.0.0" || dep(t, e, "Bad Name").Latest != "" {
		t.Fatalf("latest: %+v", e.Dependencies)
	}
	ecos = Scan(root, Options{})
	l.Fill(context.Background(), root, ecos, nil)
	data, _ := os.ReadFile(calls)
	if n := strings.Count(string(data), "view"); n != 2 {
		t.Fatalf("npm view runs once per valid name and is cached, ran %d times:\n%s", n, data)
	}
	if e, _ := Find(ecos, "node"); dep(t, e, "react").Latest != "19.1.0" {
		t.Fatal("cached answer is used")
	}
}

func TestUpgradeDescription(t *testing.T) {
	e := Ecosystem{Ecosystem: "node", Manager: "pnpm", Manifest: "web/package.json", Lockfile: "pnpm-lock.yaml", Dir: "."}
	d := UpgradeDescription(e, Dependency{Name: "react", Wanted: "^18.2.0", Installed: "18.3.1"}, "19.1.0", "shop-web", "keep IE shims")
	for _, want := range []string{"from 18.3.1 to 19.1.0", "`pnpm add react@19.1.0`", "react-dom", "breaking change", "checks", "Operator note: keep IE shims", "pnpm-lock.yaml"} {
		if !strings.Contains(d, want) {
			t.Fatalf("description lacks %q:\n%s", want, d)
		}
	}
	if UpgradeTitle("react", "19.1.0", "shop-web") != "Upgrade react to 19.1.0 in shop-web" {
		t.Fatal("title")
	}
	if ValidUpgrade("react", "19.1.0\n; rm -rf /") == nil || ValidUpgrade("--global", "1") == nil || ValidUpgrade("@types/react", "^19.0.0") != nil {
		t.Fatal("validation")
	}
}
