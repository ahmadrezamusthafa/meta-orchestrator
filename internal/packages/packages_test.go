package packages

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidation(t *testing.T) {
	bad := map[string]string{
		"unknown kind":     `[{"id":"a","name":"A","category":"Agents","kind":"curl-bash","package":"x","binaries":["a"]}]`,
		"bad category":     `[{"id":"a","name":"A","category":"Misc","kind":"npm-cli","package":"x","binaries":["a"]}]`,
		"bad id":           `[{"id":"A/../b","name":"A","category":"Agents","kind":"npm-cli","package":"x","binaries":["a"]}]`,
		"unsafe binary":    `[{"id":"a","name":"A","category":"Agents","kind":"npm-cli","package":"x","binaries":["../a"]}]`,
		"unknown requires": `[{"id":"a","name":"A","category":"Agents","kind":"npm-cli","package":"x","binaries":["a"],"requires":["zz"]}]`,
		"cycle": `[{"id":"a","name":"A","category":"Agents","kind":"npm-cli","package":"x","binaries":["a"],"requires":["b"]},
		           {"id":"b","name":"B","category":"Agents","kind":"npm-cli","package":"y","binaries":["b"],"requires":["a"]}]`,
		"wildcard home":   `[{"id":"a","name":"A","category":"Agents","kind":"npx-installer","package":"x","footprint":["~/*"],"version_file":{"path":"~/x/V","regex":"(.*)"}}]`,
		"dotdot":          `[{"id":"a","name":"A","category":"Agents","kind":"npx-installer","package":"x","footprint":["~/../etc"],"version_file":{"path":"~/x/V","regex":"(.*)"}}]`,
		"absolute":        `[{"id":"a","name":"A","category":"Agents","kind":"npx-installer","package":"x","footprint":["/etc/passwd"],"version_file":{"path":"~/x/V","regex":"(.*)"}}]`,
		"plugin no mkt":   `[{"id":"a","name":"A","category":"Plugins","kind":"claude-plugin","package":"superpowers"}]`,
		"go needs module": `[{"id":"a","name":"A","category":"Agents","kind":"go-cli","package":"x.com/a","binaries":["a"]}]`,
	}
	for name, raw := range bad {
		if _, err := ParseManifest([]byte(raw)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}

	m, err := LoadManifest(filepath.Join("..", "..", "configs", "packages.json"))
	if err != nil {
		t.Fatalf("configs/packages.json: %v", err)
	}
	want := map[string]string{"claude-code": KindNPMCLI, "node": KindSystem, "git": KindSystem, "playwright": KindNPMCLI,
		"ffmpeg": KindSystem, "docker": KindSystem, "bmad": KindNPXInstaller, "gsd": KindNPXInstaller,
		"superpowers": KindClaudePlugin, "github-mcp-server": KindGoCLI, "mcp-grafana": KindGoCLI, "go": KindSystem,
		// Launch readiness tools
		"gitleaks": KindSystem, "trivy": KindSystem, "semgrep": KindSystem, "osv-scanner": KindSystem, "syft": KindSystem,
		"checkov": KindSystem, "actionlint": KindSystem, "hadolint": KindSystem, "promtool": KindSystem, "k6": KindSystem, "spectral": KindNPMCLI,
		"govulncheck": KindGoCLI}
	if len(m.Packages) != len(want) {
		t.Fatalf("manifest has %d packages, want %d", len(m.Packages), len(want))
	}
	for id, kind := range want {
		if s := m.Get(id); s == nil || s.Kind != kind {
			t.Errorf("%s = %+v", id, s)
		}
	}
	for _, id := range []string{"claude-code", "node", "git"} {
		if !m.Get(id).Required {
			t.Errorf("%s must be required", id)
		}
	}
	// --action update with --modules fails on an existing install; quick-update keeps every module.
	if got := strings.Join(m.Get("bmad").UpdateArgs, " "); !strings.Contains(got, "--action quick-update") || strings.Contains(got, "--modules") {
		t.Errorf("bmad update_args = %q", got)
	}
	if o, _ := m.order([]string{"github-mcp-server"}); strings.Join(o, ",") != "go,github-mcp-server" {
		t.Errorf("go-cli packages must install Go first: %v", o)
	}
	if m.Get("tree-sitter") != nil {
		t.Error("tree-sitter is no longer used")
	}
	order, _ := m.order([]string{"superpowers"})
	if strings.Join(order, ",") != "node,claude-code,superpowers" {
		t.Errorf("order = %v", order)
	}
}

func TestValidateVersion(t *testing.T) {
	for _, v := range []string{"1.2.3", "v1.2.3", "6.12.4", "1.0.0-beta.1", "1.0.0+build_7"} {
		if ValidateVersion(v) != nil {
			t.Errorf("%q rejected", v)
		}
	}
	for _, v := range []string{"../x", "1.0/2", "-rf", ".hidden", "1..2", "a b", "1;rm", strings.Repeat("1", 65)} {
		if ValidateVersion(v) == nil {
			t.Errorf("%q accepted", v)
		}
	}
}

func TestNPMCLIVersionsShimsRollbackAndUninstall(t *testing.T) {
	e := newEnv(t, testManifest, "npm")

	vs, latest, err := e.m.Versions(context.Background(), "cli", false)
	if err != nil || latest != "2.0.0" || strings.Join(vs, ",") != "2.0.0,1.0.0" {
		t.Fatalf("versions = %v %q %v", vs, latest, err)
	}
	if vs, _, _ := e.m.Versions(context.Background(), "cli", true); vs[0] != "2.1.0-beta.1" {
		t.Fatalf("prerelease versions = %v", vs)
	}

	if p := e.pkg("cli"); p.Status != StatusNotInstalled || !p.Actions.Install || p.Actions.Rollback || p.Actions.Uninstall {
		t.Fatalf("before install = %+v", p)
	}
	e.ok(e.m.Install("cli", "1.0.0"))
	shim := filepath.Join(e.home, "bin", "fakecli")
	target, err := os.Readlink(shim)
	if err != nil || target != filepath.Join(e.home, "packages", "cli", "versions", "1.0.0", "node_modules", ".bin", "fakecli") {
		t.Fatalf("shim → %q %v", target, err)
	}
	if !strings.Contains(e.fakeLog(), "fakecli install chromium") {
		t.Fatalf("post_install did not run through the shim:\n%s", e.fakeLog())
	}
	p := e.pkg("cli")
	if p.Status != StatusInstalled || p.Version != "1.0.0" || !p.Managed || p.Location != shim || p.SystemCopy != "" {
		t.Fatalf("after install = %+v", p)
	}

	// Install without a version takes the latest release (prereleases skipped).
	j := e.ok(e.m.Install("cli", ""))
	if j.Version != "2.0.0" || j.FromVersion != "1.0.0" {
		t.Fatalf("job = %+v", j)
	}
	p = e.pkg("cli")
	if p.Version != "2.0.0" || !p.Actions.Rollback || len(p.InstalledVersions) != 2 || !p.InstalledVersions[0].Active || p.InstalledVersions[1].Active {
		t.Fatalf("after update = %+v", p)
	}

	// Rollback is a shim switch: npm is not asked to install anything.
	installs := strings.Count(e.fakeLog(), "npm install")
	j = e.ok(e.m.Rollback("cli", ""))
	if j.Version != "1.0.0" || strings.Count(e.fakeLog(), "npm install") != installs || !strings.Contains(j.Message, "no download") {
		t.Fatalf("rollback = %+v", j)
	}
	if p = e.pkg("cli"); p.Version != "1.0.0" {
		t.Fatalf("after rollback = %+v", p)
	}
	if p.History[0].Action != ActionRollback || p.History[0].From != "2.0.0" || p.History[0].To != "1.0.0" || !p.History[0].OK {
		t.Fatalf("history = %+v", p.History)
	}

	// A rollback to a version that is not kept installs it.
	e.ok(e.m.Rollback("cli", "0.9.0"))
	if p = e.pkg("cli"); p.Version != "0.9.0" {
		t.Fatalf("after rollback to 0.9.0 = %+v", p)
	}

	// A system copy on PATH is reported but never touched.
	e.writeScript("fakecli", "#!/bin/sh\necho 'fakecli 0.1.0'\n")
	e.m.Invalidate()
	if p = e.pkg("cli"); p.SystemCopy != filepath.Join(e.fakeBin, "fakecli") || !p.Managed || p.Version != "0.9.0" {
		t.Fatalf("with a system copy = %+v", p)
	}
	j = e.ok(e.m.Uninstall("cli"))
	if exists(shim) || exists(filepath.Join(e.home, "packages", "cli", "versions")) || !exists(filepath.Join(e.fakeBin, "fakecli")) {
		t.Fatalf("uninstall left managed files or removed the system copy")
	}
	if !strings.Contains(j.Message, "system copy") {
		t.Fatalf("uninstall message = %q", j.Message)
	}
	if p = e.pkg("cli"); p.Managed || p.Status != StatusInstalled || p.Version != "0.1.0" || p.Location != filepath.Join(e.fakeBin, "fakecli") {
		t.Fatalf("after uninstall = %+v", p)
	}
	if _, err := e.m.Uninstall("cli"); !errors.Is(err, ErrNothingManaged) {
		t.Fatalf("second uninstall = %v", err)
	}
	if _, err := e.m.Rollback("cli", ""); !errors.Is(err, ErrNothingToRollback) {
		t.Fatalf("rollback with nothing kept = %v", err)
	}
}

func TestNPMCLIFailedInstallKeepsActiveVersionAndPrunes(t *testing.T) {
	e := newEnv(t, testManifest, "npm")
	e.ok(e.m.Install("cli", "1.0.0"))
	j := e.wait(e.m.Install("cli", "9.9.9"))
	if j.State != JobFailed || !strings.Contains(strings.Join(j.Log, "\n"), "simulated failure") {
		t.Fatalf("failing install = %+v", j)
	}
	if exists(filepath.Join(e.home, "packages", "cli", "versions", "9.9.9")) {
		t.Fatal("a failed install left its folder")
	}
	if p := e.pkg("cli"); p.Version != "1.0.0" || !p.Managed {
		t.Fatalf("the active version changed after a failed install: %+v", p)
	}
	for _, v := range []string{"1.1.0", "1.2.0", "1.3.0", "1.4.0", "1.5.0"} {
		e.ok(e.m.Install("cli", v))
	}
	p := e.pkg("cli")
	if len(p.InstalledVersions) != 1+keepInactive {
		t.Fatalf("kept %d versions: %+v", len(p.InstalledVersions), p.InstalledVersions)
	}
	if exists(filepath.Join(e.home, "packages", "cli", "versions", "1.0.0")) || exists(filepath.Join(e.home, "packages", "cli", "versions", "1.1.0")) {
		t.Fatal("old versions were not pruned")
	}
}

func TestSwitchShimIsAtomic(t *testing.T) {
	dir := t.TempDir()
	if err := switchShim(dir, "x", "/a"); err != nil {
		t.Fatal(err)
	}
	if err := switchShim(dir, "x", "/b"); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.Readlink(filepath.Join(dir, "x")); got != "/b" {
		t.Fatalf("shim → %q", got)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temporary links left behind: %v", entries)
	}
	// The switch replaces the link in one rename, so a reader never finds it missing.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 200; i++ {
			_ = switchShim(dir, "x", []string{"/a", "/b"}[i%2])
		}
	}()
	missing := 0
	for running := true; running; {
		select {
		case <-done:
			running = false
		default:
			if _, err := os.Lstat(filepath.Join(dir, "x")); os.IsNotExist(err) {
				missing++
			}
		}
	}
	if missing > 0 {
		t.Fatalf("shim missing %d times during switches", missing)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("temporary links left behind: %v", entries)
	}
}

func TestGoCLI(t *testing.T) {
	e := newEnv(t, testManifest, "go")
	vs, latest, err := e.m.Versions(context.Background(), "gotool", false)
	if err != nil || latest != "1.1.0" || strings.Join(vs, ",") != "1.1.0,1.0.0" {
		t.Fatalf("versions = %v %q %v", vs, latest, err)
	}
	e.ok(e.m.Install("gotool", "1.0.0"))
	if !strings.Contains(e.fakeLog(), "go install example.com/gotool/cmd/gotool@v1.0.0") {
		t.Fatalf("go install not run with the v-prefixed version:\n%s", e.fakeLog())
	}
	// go install builds print no release number, so the version is the one installed.
	p := e.pkg("gotool")
	if p.Status != StatusInstalled || p.Version != "1.0.0" || !p.Managed {
		t.Fatalf("after install = %+v", p)
	}
	e.ok(e.m.Update("gotool"))
	if p = e.pkg("gotool"); p.Version != "1.1.0" {
		t.Fatalf("after update = %+v", p)
	}
	e.ok(e.m.Rollback("gotool", "1.0.0"))
	if p = e.pkg("gotool"); p.Version != "1.0.0" {
		t.Fatalf("after rollback = %+v", p)
	}
}

func TestNPXInstallerSnapshotsAndRestoresExactly(t *testing.T) {
	e := newEnv(t, testManifest, "npx", "npm")
	e.ok(e.m.Install("tool", "1.0.0"))
	if !strings.Contains(e.fakeLog(), "npx -y fake-tool@1.0.0 install --directory "+e.userHome) {
		t.Fatalf("installer args:\n%s", e.fakeLog())
	}
	p := e.pkg("tool")
	if p.Status != StatusInstalled || p.Version != "1.0.0" || len(p.InstalledVersions) != 1 {
		t.Fatalf("after install = %+v", p)
	}
	// An unrelated skill next to the footprint must survive everything.
	other := filepath.Join(e.userHome, ".claude", "skills", "other", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(other), 0o755)
	_ = os.WriteFile(other, []byte("mine"), 0o644)

	e.ok(e.m.Install("tool", "2.0.0"))
	if !strings.Contains(e.fakeLog(), "--action update") {
		t.Fatalf("an existing install uses update_args:\n%s", e.fakeLog())
	}
	p = e.pkg("tool")
	if p.Version != "2.0.0" || len(p.InstalledVersions) != 2 || p.InstalledVersions[1].Kind != "snapshot" || p.InstalledVersions[1].Version != "1.0.0" || !p.Actions.Rollback {
		t.Fatalf("after update = %+v", p)
	}

	npxRuns := strings.Count(e.fakeLog(), "npx ")
	e.ok(e.m.Rollback("tool", ""))
	if strings.Count(e.fakeLog(), "npx ") != npxRuns {
		t.Fatal("rollback ran the installer")
	}
	if e.read("tool/VERSION") != "1.0.0\n" || e.read(".claude/skills/tool-a/SKILL.md") != "skill 1.0.0\n" {
		t.Fatal("rollback did not restore 1.0.0")
	}
	if exists(filepath.Join(e.userHome, "tool", "new-in-2")) {
		t.Fatal("rollback kept a file 1.0.0 did not have")
	}
	if e.read(".claude/skills/other/SKILL.md") != "mine" {
		t.Fatal("rollback touched a file outside the footprint")
	}
	if p = e.pkg("tool"); p.Version != "1.0.0" {
		t.Fatalf("after rollback = %+v", p)
	}
	// Rolling forward again uses the snapshot taken before the rollback.
	e.ok(e.m.Rollback("tool", "2.0.0"))
	if e.read("tool/VERSION") != "2.0.0\n" || !exists(filepath.Join(e.userHome, "tool", "new-in-2")) {
		t.Fatal("roll forward did not restore 2.0.0")
	}

	j := e.ok(e.m.Uninstall("tool"))
	if exists(filepath.Join(e.userHome, "tool")) || exists(filepath.Join(e.userHome, ".claude", "skills", "tool-a")) || !strings.Contains(e.fakeLog(), "fake-tool@2.0.0 --uninstall") {
		t.Fatalf("uninstall = %+v\n%s", j, e.fakeLog())
	}
	if e.pkg("tool").Status != StatusNotInstalled {
		t.Fatal("still installed")
	}
	e.ok(e.m.Rollback("tool", ""))
	if e.read("tool/VERSION") != "2.0.0\n" {
		t.Fatal("rollback after uninstall did not restore")
	}
	sn := e.m.store.get("tool").Snapshots
	if len(sn) > keepSnapshots {
		t.Fatalf("%d snapshots kept", len(sn))
	}
	entries, _ := os.ReadDir(filepath.Join(e.home, "packages", "tool", "snapshots"))
	if len(entries) != len(sn) {
		t.Fatalf("snapshot files %d, state %d", len(entries), len(sn))
	}
}

func TestNPXInstallerUnknownVersion(t *testing.T) {
	e := newEnv(t, testManifest)
	_ = os.MkdirAll(filepath.Join(e.userHome, "tool"), 0o755)
	_ = os.WriteFile(filepath.Join(e.userHome, "tool", "VERSION"), []byte("\n"), 0o644)
	if p := e.pkg("tool"); p.Status != StatusInstalled || p.Version != VersionUnknown || p.UpdateAvailable {
		t.Fatalf("unreadable version = %+v", p)
	}
}

// tarball builds a snapshot archive with the given entries.
func tarball(t *testing.T, path string, hdrs []*tar.Header) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, h := range hdrs {
		if h.Typeflag == tar.TypeReg {
			h.Size = 1
		}
		h.Mode = 0o644
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte("x"))
		}
	}
	tw.Close()
	gz.Close()
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotRestoreRefusesUnsafeEntries(t *testing.T) {
	root := filepath.Join(t.TempDir(), "home")
	outside := filepath.Join(filepath.Dir(root), "outside")
	_ = os.MkdirAll(filepath.Join(root, "tool"), 0o755)
	_ = os.MkdirAll(outside, 0o755)
	_ = os.WriteFile(filepath.Join(root, "tool", "keep"), []byte("current"), 0o644)
	_ = os.Symlink(outside, filepath.Join(root, "escape"))

	cases := map[string][]*tar.Header{
		"dotdot":       {{Name: "../outside/pwned", Typeflag: tar.TypeReg}},
		"nested":       {{Name: "tool/../../outside/pwned", Typeflag: tar.TypeReg}},
		"absolute":     {{Name: "/tmp/pwned", Typeflag: tar.TypeReg}},
		"link escape":  {{Name: "tool/link", Typeflag: tar.TypeSymlink, Linkname: "../../outside"}},
		"link abs":     {{Name: "tool/link", Typeflag: tar.TypeSymlink, Linkname: "/etc"}},
		"via symlink":  {{Name: "escape/pwned", Typeflag: tar.TypeReg}},
		"device":       {{Name: "tool/dev", Typeflag: tar.TypeChar}},
		"hardlink out": {{Name: "tool/hard", Typeflag: tar.TypeLink, Linkname: "../outside/x"}},
	}
	for name, hdrs := range cases {
		src := filepath.Join(t.TempDir(), "s.tar.gz")
		tarball(t, src, hdrs)
		err := restoreSnapshot(src, root, []string{"tool"})
		if !errors.Is(err, ErrUnsafeArchive) {
			t.Errorf("%s: err = %v", name, err)
		}
		if raw, _ := os.ReadFile(filepath.Join(root, "tool", "keep")); string(raw) != "current" {
			t.Fatalf("%s: current files were removed before the archive was validated", name)
		}
		if exists(filepath.Join(outside, "pwned")) {
			t.Fatalf("%s: wrote outside the home folder", name)
		}
	}

	// A footprint pattern never matches outside root, and relative links inside it are kept.
	_ = os.Symlink("keep", filepath.Join(root, "tool", "rel"))
	rels, err := resolveFootprint([]string{"~/tool"}, root, root)
	if err != nil || len(rels) != 1 {
		t.Fatalf("footprint = %v %v", rels, err)
	}
	src := filepath.Join(t.TempDir(), "ok.tar.gz")
	if err := writeSnapshot(src, root, rels); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "tool", "extra"), []byte("new"), 0o644)
	if err := restoreSnapshot(src, root, rels); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(root, "tool", "extra")) {
		t.Fatal("restore kept a file the snapshot lacks")
	}
	if l, err := os.Readlink(filepath.Join(root, "tool", "rel")); err != nil || l != "keep" {
		t.Fatalf("relative link = %q %v", l, err)
	}
}

func TestClaudePluginUpdateRollbackUninstall(t *testing.T) {
	e := newEnv(t, testManifest, "claude")
	if _, err := e.m.Install("plug", "1.0.0"); !errors.Is(err, ErrVersionNotPinnable) {
		t.Fatalf("pinned plugin install = %v", err)
	}
	e.ok(e.m.Install("plug", ""))
	p := e.pkg("plug")
	if p.Status != StatusInstalled || p.Version != "1.0.0" || p.Actions.PickVersion || p.Actions.Rollback || p.Actions.Install {
		t.Fatalf("after install = %+v", p)
	}
	if !strings.Contains(e.fakeLog(), "claude plugin install plug@mkt --scope user") {
		t.Fatalf("log:\n%s", e.fakeLog())
	}
	e.ok(e.m.Update("plug"))
	p = e.pkg("plug")
	if p.Version != "2.0.0" || !p.Actions.Rollback || !strings.Contains(p.ActionNotes.Rollback, "Best effort") {
		t.Fatalf("after update = %+v", p)
	}
	if exists(filepath.Join(e.userHome, ".claude", "plugins", "cache", "mkt", "plug", "1.0.0")) {
		t.Fatal("the fake update should have removed the old cache folder")
	}

	j := e.ok(e.m.Rollback("plug", ""))
	if !strings.Contains(j.Message, "Restart Claude Code") {
		t.Fatalf("rollback message = %q", j.Message)
	}
	pluginsFile := filepath.Join(e.userHome, ".claude", "plugins", "installed_plugins.json")
	var f struct {
		Version int                          `json:"version"`
		Plugins map[string][]pluginListEntry `json:"plugins"`
	}
	raw, _ := os.ReadFile(pluginsFile)
	if err := json.Unmarshal(raw, &f); err != nil || f.Version != 2 || f.Plugins["plug@mkt"][0].Version != "1.0.0" {
		t.Fatalf("installed_plugins.json = %s", raw)
	}
	if bak, err := os.ReadFile(pluginsFile + ".bak"); err != nil || !strings.Contains(string(bak), `"2.0.0"`) {
		t.Fatalf(".bak = %s %v", bak, err)
	}
	if e.read(".claude/plugins/cache/mkt/plug/1.0.0/README.md") != "plugin 1.0.0\n" {
		t.Fatal("cache folder not restored")
	}
	if p = e.pkg("plug"); p.Version != "1.0.0" {
		t.Fatalf("after rollback = %+v", p)
	}

	e.ok(e.m.Uninstall("plug"))
	if p = e.pkg("plug"); p.Status != StatusNotInstalled || !p.Actions.Rollback {
		t.Fatalf("after uninstall = %+v", p)
	}
	e.ok(e.m.Rollback("plug", ""))
	if p = e.pkg("plug"); p.Status != StatusInstalled || p.Version != "1.0.0" || !strings.Contains(e.fakeLog(), "claude plugin enable plug@mkt") {
		t.Fatalf("rollback after uninstall = %+v\n%s", p, e.fakeLog())
	}
	if _, err := e.m.Rollback("plug", "7.7.7"); !errors.Is(err, ErrNothingToRollback) {
		t.Fatalf("rollback to a version without snapshot = %v", err)
	}
}

func TestSystemPackages(t *testing.T) {
	e := newEnv(t, testManifest)
	_, err := e.m.Install("ff", "")
	var me *ManualError
	if !errors.As(err, &me) || !containsString(me.Manual.Commands, "brew install fakeff") {
		t.Fatalf("install without brew = %v", err)
	}
	if p := e.pkg("ff"); p.Actions.Install || p.Manual == nil || p.ActionNotes.Rollback == "" {
		t.Fatalf("without brew = %+v", p)
	}

	e.writeScript("brew", fakeBrew)
	e.m.Invalidate()
	if _, err := e.m.Install("ff", "1.0"); !errors.Is(err, ErrVersionNotPinnable) {
		t.Fatalf("pinned system install = %v", err)
	}
	e.ok(e.m.Install("ff", ""))
	p := e.pkg("ff")
	if p.Status != StatusInstalled || p.Version != "1.0" || p.Managed || !p.Actions.Update || p.Actions.Rollback || p.Manual != nil {
		t.Fatalf("after install = %+v", p)
	}
	e.ok(e.m.Update("ff"))
	if p = e.pkg("ff"); p.Version != "1.1" {
		t.Fatalf("after update = %+v", p)
	}
	if _, err := e.m.Rollback("ff", ""); !errors.Is(err, ErrRollbackUnsupported) {
		t.Fatalf("system rollback = %v", err)
	}
	e.ok(e.m.Uninstall("ff"))
	if e.pkg("ff").Status != StatusNotInstalled {
		t.Fatal("still installed")
	}
}

func TestJobLocking(t *testing.T) {
	e := newEnv(t, `[
	  {"id": "a", "name": "A", "category": "Agents", "kind": "npm-cli", "package": "pa", "binaries": ["fakecli"]},
	  {"id": "b", "name": "B", "category": "Agents", "kind": "npm-cli", "package": "pb", "binaries": ["fakecli2"]}]`, "npm")
	t.Setenv("FAKE_BINS", "fakecli fakecli2")
	t.Setenv("FAKE_SLEEP", "0.3")
	ja, err := e.m.Install("a", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if busy, err := e.m.Install("a", "2.0.0"); !errors.Is(err, ErrBusy) || busy.ID != ja.ID {
		t.Fatalf("second job for a = %v %v", busy, err)
	}
	jb, err := e.m.Install("b", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	e.ok(ja, nil)
	got := e.ok(jb, nil)
	if !strings.Contains(strings.Join(got.Log, "\n"), "Waiting for another npm job") {
		t.Fatalf("b did not wait for the npm lock: %v", got.Log)
	}
	var seq []string
	for _, l := range strings.Split(e.fakeLog(), "\n") {
		if strings.HasPrefix(l, "start ") || strings.HasPrefix(l, "end ") {
			seq = append(seq, strings.Fields(l)[0])
		}
	}
	if strings.Join(seq, ",") != "start,end,start,end" {
		t.Fatalf("npm runs overlapped: %v", seq)
	}
	// Jobs were published, the last copy finished.
	var last Job
	for len(e.jobs) > 0 {
		if j := <-e.jobs; j.ID == jb.ID {
			last = j
		}
	}
	if last.State != JobSucceeded {
		t.Fatalf("last published job = %+v", last)
	}
}

func TestStatePersistsAcrossRestarts(t *testing.T) {
	e := newEnv(t, testManifest, "npm")
	e.ok(e.m.Install("cli", "1.0.0"))
	e.ok(e.m.Install("cli", "2.0.0"))
	info, err := os.Stat(filepath.Join(e.home, "packages", "state.json"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("state file = %v %v", info, err)
	}
	m2, err := New(e.m.Manifest(), Options{Home: e.home, UserHome: e.userHome})
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()
	p, _ := m2.Get("cli")
	if p.Version != "2.0.0" || len(p.InstalledVersions) != 2 || len(p.History) != 2 || p.History[0].To != "2.0.0" {
		t.Fatalf("after restart = %+v", p)
	}
	if _, err := m2.Rollback("cli", ""); err != nil {
		t.Fatalf("rollback after restart: %v", err)
	}
}

func TestSetupInstallsRequiredInOrderAndStopsOnFailure(t *testing.T) {
	e := newEnv(t, testManifest, "npm", "brew")
	res, err := e.m.Setup()
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Jobs) != 2 || res.Jobs[0].Package != "node" || res.Jobs[1].Package != "cli" || res.Jobs[0].Action != ActionSetup {
		t.Fatalf("setup jobs = %+v", res.Jobs)
	}
	for _, j := range res.Jobs {
		e.ok(&j, nil)
	}
	if p := e.pkg("cli"); p.Status != StatusInstalled || p.Version != "2.0.0" {
		t.Fatalf("cli = %+v", p)
	}
	res, _ = e.m.Setup()
	if len(res.Jobs) != 0 || len(res.Skipped) != 2 || !strings.HasPrefix(res.Skipped[0].Reason, "already installed") {
		t.Fatalf("second setup = %+v", res)
	}

	f := newEnv(t, `[
	  {"id": "base", "name": "Base", "category": "Runtimes", "kind": "npm-cli", "package": "fail-pkg", "binaries": ["fakebase"], "required": true},
	  {"id": "top", "name": "Top", "category": "Agents", "kind": "npm-cli", "package": "top", "binaries": ["fakecli"], "required": true, "requires": ["base"]}]`, "npm")
	res, _ = f.m.Setup()
	if len(res.Jobs) != 2 || res.Jobs[0].Package != "base" {
		t.Fatalf("jobs = %+v", res.Jobs)
	}
	first := f.wait(&res.Jobs[0], nil)
	second := f.wait(&res.Jobs[1], nil)
	if first.State != JobFailed || second.State != JobFailed || !strings.Contains(second.Message, "setup stopped") {
		t.Fatalf("chain = %+v / %+v", first, second)
	}
	if strings.Contains(f.fakeLog(), "top@") {
		t.Fatal("the chain ran a package after a failure")
	}
}

func TestListDoesNotWaitForTheNetwork(t *testing.T) {
	e := newEnv(t, testManifest, "npm")
	e.writeScript("npm", "#!/bin/sh\nsleep 2\necho '[\"3.0.0\"]'\n")
	l := e.m.List()
	if len(l.Packages) != 6 || !l.OnPath || l.BinDir != filepath.Join(e.home, "bin") {
		t.Fatalf("listing = %+v", l)
	}
	for _, p := range l.Packages {
		if p.Latest != "" {
			t.Fatalf("%s latest = %q before any fetch finished", p.ID, p.Latest)
		}
	}
}

func TestEnsurePathPrependsBin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("PATH", "/usr/bin"+string(os.PathListSeparator)+filepath.Join(home, "bin"))
	bin := EnsurePath(home)
	parts := filepath.SplitList(os.Getenv("PATH"))
	if parts[0] != bin || len(parts) != 2 || !exists(bin) {
		t.Fatalf("PATH = %v", parts)
	}
}

func TestCLI(t *testing.T) {
	e := newEnv(t, testManifest, "npm")
	var out bytes.Buffer
	if code := RunCLI(context.Background(), e.m, []string{"install", "cli", "1.0.0"}, &out); code != 0 {
		t.Fatalf("install exit %d:\n%s", code, out.String())
	}
	if !strings.Contains(out.String(), "[cli] $ npm install --prefix") || !strings.Contains(out.String(), "[cli] succeeded") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	if code := RunCLI(context.Background(), e.m, []string{"list"}, &out); code != 0 || !strings.Contains(out.String(), "cli") || !strings.Contains(out.String(), "1.0.0") {
		t.Fatalf("list exit %d:\n%s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(context.Background(), e.m, []string{"versions", "cli"}, &out); code != 0 || !strings.Contains(out.String(), "latest: 2.0.0") {
		t.Fatalf("versions exit %d:\n%s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(context.Background(), e.m, []string{"install", "cli", "9.9.9"}, &out); code != 1 || !strings.Contains(out.String(), "failed") {
		t.Fatalf("failing install exit %d:\n%s", code, out.String())
	}
	out.Reset()
	if code := RunCLI(context.Background(), e.m, []string{"rollback", "ff"}, &out); code != 1 || !strings.Contains(out.String(), "not supported") {
		t.Fatalf("system rollback exit %d:\n%s", code, out.String())
	}
	if code := RunCLI(context.Background(), e.m, []string{"bogus"}, &out); code != 2 {
		t.Fatalf("unknown command exit %d", code)
	}
	if code := RunCLI(context.Background(), e.m, []string{"install"}, &out); code != 2 {
		t.Fatalf("missing id exit %d", code)
	}
}

func TestFootprintFromInstallerManifest(t *testing.T) {
	home := t.TempDir()
	skills := filepath.Join(home, ".claude", "skills")
	for _, d := range []string{"bmad-dev", "wds-flow", "memory", "gsd-plan"} {
		if err := os.MkdirAll(filepath.Join(skills, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(home, "_bmad", "_config"), 0o755); err != nil {
		t.Fatal(err)
	}
	csvBody := "canonicalId,name\nbmad-dev,a\nwds-flow,b\nmemory,c\n../escape,d\nx/y,e\n"
	if err := os.WriteFile(filepath.Join(home, "_bmad", "_config", "skill-manifest.csv"), []byte(csvBody), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Spec{Footprint: []string{"{dir}/_bmad"}, FootprintFrom: &FootprintList{
		CSV: "{dir}/_bmad/_config/skill-manifest.csv", Column: "canonicalId", Prefix: "{dir}/.claude/skills"}}
	rels, err := resolveFootprint(footprintPatterns(s, home, home), home, home)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(rels, ",")
	want := strings.Join([]string{filepath.Join(".claude", "skills", "bmad-dev"), filepath.Join(".claude", "skills", "memory"),
		filepath.Join(".claude", "skills", "wds-flow"), "_bmad"}, ",")
	if got != want {
		t.Fatalf("footprint = %s, want %s (gsd-plan and unsafe names must be left out)", got, want)
	}
}

func TestSystemUpdateRefusesCopiesHomebrewDoesNotOwn(t *testing.T) {
	e := newEnv(t, testManifest)
	e.writeScript("brew", fakeBrew)
	e.writeScript("fakeff", "#!/bin/sh\necho 'fakeff version 0.9'\n") // e.g. a vendor installer's copy
	e.m.Invalidate()
	p := e.pkg("ff")
	if p.Status != StatusInstalled || p.Actions.Update || p.Actions.Uninstall || !strings.Contains(p.ActionNotes.Update, "not installed by Homebrew") {
		t.Fatalf("a non-Homebrew copy must not be offered brew upgrade: %+v", p)
	}
	var me *ManualError
	if _, err := e.m.Update("ff"); !errors.As(err, &me) {
		t.Fatalf("update of a non-Homebrew copy = %v", err)
	}
}
