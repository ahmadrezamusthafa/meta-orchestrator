package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fake npm on PATH "installs" a CLI that prints its version; nothing is installed for real.
const apiFakeNPM = `#!/bin/sh
case "$1" in
view) echo '["1.0.0","1.1.0","2.0.0-rc.1"]' ;;
install)
  prefix="$3"; spec="$6"; ver="${spec##*@}"
  mkdir -p "$prefix/node_modules/.bin"
  printf '#!/bin/sh\necho "apicli %s"\n' "$ver" > "$prefix/node_modules/.bin/apicli"
  chmod +x "$prefix/node_modules/.bin/apicli" ;;
esac
`

const apiPackagesManifest = `[
  {"id": "apicli", "name": "API CLI", "category": "Agents", "kind": "npm-cli", "package": "apicli", "binaries": ["apicli"], "required": true},
  {"id": "media", "name": "Media", "category": "Media", "kind": "system", "binaries": ["fake-media-tool"], "system": {"brew": "fake-media-tool"}}
]`

// packagesRouter serves a manifest from RootDir with MO_HOME, HOME and PATH in temp folders.
func packagesRouter(t *testing.T) (*Router, string) {
	t.Helper()
	root, tmp := t.TempDir(), t.TempDir()
	moHome := filepath.Join(tmp, "mo")
	fakeBin := filepath.Join(tmp, "fakebin")
	for _, d := range []string{filepath.Join(root, "configs"), fakeBin, filepath.Join(tmp, "home")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "packages.json"), []byte(apiPackagesManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakeBin, "npm"), []byte(apiFakeNPM), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MO_HOME", moHome)
	t.Setenv("HOME", filepath.Join(tmp, "home"))
	t.Setenv("PATH", strings.Join([]string{filepath.Join(moHome, "bin"), fakeBin, "/usr/bin", "/bin"}, string(os.PathListSeparator)))
	r := NewRouter(RouterConfig{RootDir: root})
	t.Cleanup(func() {
		if m := r.packageManager(); m != nil {
			m.Close()
		}
	})
	return r, moHome
}

func packageByID(t *testing.T, out map[string]interface{}, id string) map[string]interface{} {
	t.Helper()
	for _, p := range out["packages"].([]interface{}) {
		if m := p.(map[string]interface{}); m["id"] == id {
			return m
		}
	}
	t.Fatalf("package %s not listed: %v", id, out)
	return nil
}

func waitPackageJob(t *testing.T, r *Router, id string) map[string]interface{} {
	t.Helper()
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		w, out := tcDo(t, r, http.MethodGet, "/api/v1/packages/jobs/"+id, "")
		if w.Code != http.StatusOK {
			t.Fatalf("GET job = %d %v", w.Code, out)
		}
		if out["state"] == "succeeded" || out["state"] == "failed" {
			return out
		}
	}
	t.Fatalf("job %s did not finish", id)
	return nil
}

func TestPackagesAPIContract(t *testing.T) {
	r, moHome := packagesRouter(t)

	w, out := tcDo(t, r, http.MethodGet, "/api/v1/packages", "")
	if w.Code != http.StatusOK || out["home"] != moHome || out["bin_dir"] != filepath.Join(moHome, "bin") || out["on_path"] != true {
		t.Fatalf("GET /packages = %d %v", w.Code, out)
	}
	p := packageByID(t, out, "apicli")
	for _, key := range []string{"id", "name", "category", "description", "docs", "kind", "package", "required", "requires", "status",
		"version", "latest", "update_available", "managed", "location", "system_copy", "installed_versions", "actions",
		"action_notes", "manual", "job", "history", "notes"} {
		if _, ok := p[key]; !ok {
			t.Errorf("package lacks %q", key)
		}
	}
	if p["status"] != "not_installed" || p["job"] != nil || p["manual"] != nil {
		t.Fatalf("apicli = %v", p)
	}
	actions := p["actions"].(map[string]interface{})
	for _, key := range []string{"install", "update", "rollback", "uninstall", "pick_version"} {
		if _, ok := actions[key]; !ok {
			t.Errorf("actions lack %q", key)
		}
	}
	if media := packageByID(t, out, "media"); media["manual"] == nil || media["actions"].(map[string]interface{})["install"] != false {
		t.Fatalf("media without brew = %v", media)
	}

	w, out = tcDo(t, r, http.MethodGet, "/api/v1/packages/apicli/versions", "")
	if w.Code != http.StatusOK || out["latest"] != "1.1.0" || len(out["versions"].([]interface{})) != 2 {
		t.Fatalf("versions = %d %v", w.Code, out)
	}
	if _, out = tcDo(t, r, http.MethodGet, "/api/v1/packages/apicli/versions?prerelease=1", ""); out["versions"].([]interface{})[0] != "2.0.0-rc.1" {
		t.Fatalf("prerelease versions = %v", out)
	}

	// Errors: bad version 400, unknown package 404, manual 501, nothing to roll back 400.
	if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/install", `{"version":"../../x"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("unsafe version = %d", w.Code)
	}
	if w, _ := tcDo(t, r, http.MethodPost, "/api/v1/packages/nope/install", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown package = %d", w.Code)
	}
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/media/install", "")
	if w.Code != http.StatusNotImplemented || out["error"] == "" || out["manual"].(map[string]interface{})["commands"] == nil {
		t.Fatalf("manual install = %d %v", w.Code, out)
	}
	if w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/media/rollback", ""); w.Code != http.StatusNotImplemented {
		t.Fatalf("system rollback = %d %v", w.Code, out)
	}
	if w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/rollback", ""); w.Code != http.StatusBadRequest {
		t.Fatalf("rollback with nothing kept = %d %v", w.Code, out)
	}
	if w, _ := tcDo(t, r, http.MethodGet, "/api/v1/packages/apicli/install", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET install = %d", w.Code)
	}

	// Install a pinned version, then the latest, then roll back.
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/install", `{"version":"1.0.0"}`)
	if w.Code != http.StatusAccepted || out["package"] != "apicli" || out["action"] != "install" || out["version"] != "1.0.0" {
		t.Fatalf("install = %d %v", w.Code, out)
	}
	for _, key := range []string{"id", "package", "action", "version", "from_version", "state", "log", "message", "started", "finished"} {
		if _, ok := out[key]; !ok {
			t.Errorf("job lacks %q", key)
		}
	}
	if w, busy := tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/install", ""); w.Code == http.StatusConflict && busy["job"] == nil {
		t.Fatalf("409 without the running job: %v", busy)
	}
	if job := waitPackageJob(t, r, out["id"].(string)); job["state"] != "succeeded" {
		t.Fatalf("install job = %v", job)
	}
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/update", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("update = %d %v", w.Code, out)
	}
	if job := waitPackageJob(t, r, out["id"].(string)); job["state"] != "succeeded" || job["version"] != "1.1.0" || job["from_version"] != "1.0.0" {
		t.Fatalf("update job = %v", job)
	}
	_, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/recheck", "")
	p = packageByID(t, out, "apicli")
	if p["status"] != "installed" || p["version"] != "1.1.0" || p["managed"] != true || len(p["installed_versions"].([]interface{})) != 2 ||
		p["actions"].(map[string]interface{})["rollback"] != true || len(p["history"].([]interface{})) != 2 {
		t.Fatalf("after update = %v", p)
	}
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/rollback", `{}`)
	if w.Code != http.StatusAccepted || out["version"] != "1.0.0" {
		t.Fatalf("rollback = %d %v", w.Code, out)
	}
	waitPackageJob(t, r, out["id"].(string))
	if _, out = tcDo(t, r, http.MethodGet, "/api/v1/packages", ""); packageByID(t, out, "apicli")["version"] != "1.0.0" {
		t.Fatalf("after rollback = %v", packageByID(t, out, "apicli"))
	}

	// Setup: apicli is installed, so nothing is left to do.
	w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/setup", "")
	if w.Code != http.StatusAccepted || len(out["jobs"].([]interface{})) != 0 || len(out["skipped"].([]interface{})) != 1 {
		t.Fatalf("setup = %d %v", w.Code, out)
	}

	w, out = tcDo(t, r, http.MethodPost, "/api/v1/packages/apicli/uninstall", "")
	if w.Code != http.StatusAccepted {
		t.Fatalf("uninstall = %d %v", w.Code, out)
	}
	waitPackageJob(t, r, out["id"].(string))
	if _, err := os.Lstat(filepath.Join(moHome, "bin", "apicli")); !os.IsNotExist(err) {
		t.Fatal("uninstall left the shim")
	}
	if w, _ := tcDo(t, r, http.MethodGet, "/api/v1/packages/jobs/nope", ""); w.Code != http.StatusNotFound {
		t.Fatalf("unknown job = %d", w.Code)
	}
	if w, _ := tcDo(t, r, http.MethodGet, "/api/v1/tools", ""); w.Code != http.StatusNotFound {
		t.Fatalf("the old /tools endpoint still answers: %d", w.Code)
	}
}
