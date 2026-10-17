package packages

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

// Every test runs against a temporary machine: a temp HOME, a temp orchestrator home and fake
// npm/go/npx/claude/brew scripts on PATH that create the files a real install would. Nothing is
// installed on the machine running the tests.

const fakeNPM = `#!/bin/sh
echo "npm $*" >> "$FAKE_LOG"
case "$1" in
view) printf '%s\n' "$FAKE_NPM_VERSIONS" ;;
install)
  prefix="$3"; spec="$6"; ver="${spec##*@}"
  echo "start $spec" >> "$FAKE_LOG"
  [ -n "$FAKE_SLEEP" ] && sleep "$FAKE_SLEEP"
  case "$spec" in fail-pkg@*|*@9.9.9) echo "npm ERR! simulated failure"; exit 1 ;; esac
  mkdir -p "$prefix/node_modules/.bin"
  for b in $FAKE_BINS; do
    printf '#!/bin/sh\nif [ "$1" = "--version" ]; then echo "%s %s"; exit 0; fi\necho "%s $*" >> "%s"\n' "$b" "$ver" "$b" "$FAKE_LOG" > "$prefix/node_modules/.bin/$b"
    chmod +x "$prefix/node_modules/.bin/$b"
  done
  echo "end $spec" >> "$FAKE_LOG"
  echo "added 1 package" ;;
esac
`

const fakeGo = `#!/bin/sh
echo "go $*" >> "$FAKE_LOG"
case "$1" in
list) echo "$4 v1.0.0 v1.1.0 v1.2.0-rc.1" ;;
install)
  spec="$2"; ver="${spec##*@}"
  mkdir -p "$GOBIN"
  printf '#!/bin/sh\necho "Version: version"\n' > "$GOBIN/gotool"
  chmod +x "$GOBIN/gotool"
  echo "installed $ver into $GOBIN" ;;
esac
`

// The fake installer owns ~/tool and ~/.claude/skills/tool-*; version 2.x adds a file 1.x lacks.
const fakeNPX = `#!/bin/sh
echo "npx $*" >> "$FAKE_LOG"
spec="$2"; ver="${spec##*@}"; shift 2
for a in "$@"; do
  if [ "$a" = "--uninstall" ]; then rm -rf "$HOME/tool" "$HOME/.claude/skills/tool-a"; echo "uninstalled"; exit 0; fi
done
echo "args: $*"
rm -rf "$HOME/tool"
mkdir -p "$HOME/tool" "$HOME/.claude/skills/tool-a"
echo "$ver" > "$HOME/tool/VERSION"
echo "skill $ver" > "$HOME/.claude/skills/tool-a/SKILL.md"
case "$ver" in 2.*) echo new > "$HOME/tool/new-in-2" ;; esac
echo "installed $ver"
`

// The fake claude derives its plugin listing from installed_plugins.json, like the real one.
const fakeClaude = `#!/bin/sh
echo "claude $*" >> "$FAKE_LOG"
F="$HOME/.claude/plugins/installed_plugins.json"
write() {
  v="$1"; dir="$HOME/.claude/plugins/cache/mkt/plug/$v"
  mkdir -p "$dir" "$HOME/.claude/plugins"
  echo "plugin $v" > "$dir/README.md"
  printf '{\n  "version": 2,\n  "plugins": {\n    "plug@mkt": [\n      {\n        "scope": "user",\n        "installPath": "%s",\n        "version": "%s"\n      }\n    ]\n  }\n}\n' "$dir" "$v" > "$F"
}
if [ "$1" = "--version" ]; then echo "2.0.0 (Claude Code)"; exit 0; fi
case "$2" in
list)
  if [ -f "$F" ] && grep -q '"plug@mkt"' "$F"; then
    v=$(grep -o '"version": *"[^"]*"' "$F" | head -1 | sed 's/.*: *"//; s/"//')
    p=$(grep -o '"installPath": *"[^"]*"' "$F" | head -1 | sed 's/.*: *"//; s/"//')
    printf '[{"id":"plug@mkt","version":"%s","scope":"user","installPath":"%s"}]\n' "$v" "$p"
  else
    echo "[]"
  fi ;;
install) write 1.0.0 ;;
update) rm -rf "$HOME/.claude/plugins/cache/mkt/plug/1.0.0"; write 2.0.0 ;;
uninstall) rm -rf "$HOME/.claude/plugins/cache/mkt/plug"; printf '{"version": 2, "plugins": {}}\n' > "$F" ;;
enable) echo "enabled" ;;
esac
`

const fakeBrew = `#!/bin/sh
echo "brew $*" >> "$FAKE_LOG"
case "$1" in
install) printf '#!/bin/sh\necho "%s version 1.0"\n' "$2" > "$FAKE_BIN/$2"; chmod +x "$FAKE_BIN/$2" ;;
upgrade) printf '#!/bin/sh\necho "%s version 1.1"\n' "$2" > "$FAKE_BIN/$2"; chmod +x "$FAKE_BIN/$2" ;;
uninstall) rm -f "$FAKE_BIN/$2" ;;
info) echo '{"formulae":[{"versions":{"stable":"1.1"}}]}' ;;
esac
`

const testManifest = `[
  {"id": "node", "name": "Node", "category": "Runtimes", "kind": "system", "binaries": ["fakenode"], "required": true, "system": {"brew": "fakenode"}},
  {"id": "cli", "name": "Fake CLI", "category": "Agents", "kind": "npm-cli", "package": "@scope/fake-cli", "binaries": ["fakecli"],
   "required": true, "requires": ["node"], "post_install": [["fakecli", "install", "chromium"]]},
  {"id": "gotool", "name": "Go Tool", "category": "Connectors", "kind": "go-cli", "package": "example.com/gotool/cmd/gotool", "module": "example.com/gotool", "binaries": ["gotool"]},
  {"id": "tool", "name": "Tool Method", "category": "Methodologies", "kind": "npx-installer", "package": "fake-tool", "scope_dir": "~",
   "install_args": ["install", "--directory", "{dir}"], "update_args": ["install", "--directory", "{dir}", "--action", "update"],
   "uninstall_args": ["--uninstall"],
   "version_file": {"path": "{dir}/tool/VERSION", "regex": "^\\s*([0-9A-Za-z.+_-]+)"},
   "footprint": ["{dir}/tool", "~/.claude/skills/tool-*"]},
  {"id": "plug", "name": "Plug", "category": "Plugins", "kind": "claude-plugin", "package": "plug@mkt", "requires": ["cli"]},
  {"id": "ff", "name": "FF", "category": "Media", "kind": "system", "binaries": ["fakeff"], "system": {"brew": "fakeff", "apt": "fakeff"}}
]`

type env struct {
	t        *testing.T
	root     string
	home     string // orchestrator home
	userHome string
	fakeBin  string
	log      string
	m        *Manager
	jobs     chan Job
}

// newEnv prepares the fake machine. tools lists the fake package managers to put on PATH.
func newEnv(t *testing.T, manifest string, tools ...string) *env {
	t.Helper()
	root := t.TempDir()
	e := &env{t: t, root: root, home: filepath.Join(root, "mo"), userHome: filepath.Join(root, "user"),
		fakeBin: filepath.Join(root, "fakebin"), log: filepath.Join(root, "fake.log"), jobs: make(chan Job, 1000)}
	for _, d := range []string{e.userHome, e.fakeBin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scripts := map[string]string{"npm": fakeNPM, "go": fakeGo, "npx": fakeNPX, "claude": fakeClaude, "brew": fakeBrew}
	for _, name := range tools {
		e.writeScript(name, scripts[name])
	}
	t.Setenv("HOME", e.userHome)
	t.Setenv("FAKE_LOG", e.log)
	t.Setenv("FAKE_BIN", e.fakeBin)
	t.Setenv("FAKE_BINS", "fakecli")
	t.Setenv("FAKE_NPM_VERSIONS", `["1.0.0","2.0.0","2.1.0-beta.1"]`)
	t.Setenv("FAKE_SLEEP", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("PATH", strings.Join([]string{filepath.Join(e.home, "bin"), e.fakeBin, "/usr/bin", "/bin"}, string(os.PathListSeparator)))
	man, err := ParseManifest([]byte(manifest))
	if err != nil {
		t.Fatal(err)
	}
	e.m, err = New(man, Options{Home: e.home, UserHome: e.userHome, Toolchain: toolchain.New(),
		OnJob: func(j Job) {
			select {
			case e.jobs <- j:
			default:
			}
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(e.m.Close)
	return e
}

func (e *env) writeScript(name, body string) {
	e.t.Helper()
	if err := os.WriteFile(filepath.Join(e.fakeBin, name), []byte(body), 0o755); err != nil {
		e.t.Fatal(err)
	}
}

// wait returns the finished job.
func (e *env) wait(j *Job, err error) *Job {
	e.t.Helper()
	if err != nil {
		e.t.Fatalf("start: %v", err)
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		cur := e.m.Job(j.ID)
		if cur.State == JobSucceeded || cur.State == JobFailed {
			return cur
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.t.Fatalf("job %s did not finish: %+v", j.ID, e.m.Job(j.ID))
	return nil
}

func (e *env) ok(j *Job, err error) *Job {
	e.t.Helper()
	got := e.wait(j, err)
	if got.State != JobSucceeded {
		e.t.Fatalf("job %s %s failed: %s\n%s", got.Package, got.Action, got.Message, strings.Join(got.Log, "\n"))
	}
	return got
}

func (e *env) pkg(id string) Package {
	e.t.Helper()
	p, err := e.m.Get(id)
	if err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) fakeLog() string {
	raw, _ := os.ReadFile(e.log)
	return string(raw)
}

func (e *env) read(rel string) string {
	e.t.Helper()
	raw, err := os.ReadFile(filepath.Join(e.userHome, rel))
	if err != nil {
		e.t.Fatalf("read %s: %v", rel, err)
	}
	return string(raw)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
