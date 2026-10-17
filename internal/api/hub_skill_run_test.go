package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeScriptSkill creates a Claude skill whose scripts/run.sh is its entrypoint.
func writeScriptSkill(t *testing.T, root, name, frontmatter, script string) {
	t.Helper()
	dir := writeSkill(t, root, name, "Script skill", "Run it.")
	if frontmatter != "" {
		md := "---\nname: " + name + "\ndescription: Script skill\n" + frontmatter + "---\nRun it.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(md), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.MkdirAll(filepath.Join(dir, "scripts"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "scripts", "run.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runSkill(t *testing.T, r *Router, name, body string) (int, skillRunResponse) {
	t.Helper()
	w := do(r, http.MethodPost, "/api/v1/skills/"+name+"/run", body)
	var out skillRunResponse
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestSkillTestRunExecutesCommandSkillsAsARole(t *testing.T) {
	root := profileRoot(t)
	writeScriptSkill(t, root, "probe-run", "", "#!/bin/sh\necho \"cwd=$(pwd)\"\ncat\n")
	writeScriptSkill(t, root, "probe-fail", "", "#!/bin/sh\necho broken >&2\nexit 3\n")
	writeScriptSkill(t, root, "probe-devops-only", "required_roles:\n  - devops_superpower\n", "#!/bin/sh\necho ran\n")
	writeScriptSkill(t, root, "probe-denied", "", "#!/bin/sh\necho ran\n")
	writeSkill(t, root, "probe-instructions", "Instruction-only", "Just guidance.")
	r := NewRouter(RouterConfig{RootDir: root})

	code, out := runSkill(t, r, "probe-run", `{"role":"lead_developer","parameters":{"x":1}}`)
	realRoot, _ := filepath.EvalSymlinks(root)
	if code != http.StatusOK || !out.OK || !strings.Contains(out.Output, "cwd="+realRoot) || !strings.Contains(out.Output, `{"x":1}`) {
		t.Fatalf("run = %d %+v", code, out)
	}
	if code, out = runSkill(t, r, "probe-fail", `{"role":"lead_developer"}`); code != http.StatusOK || out.OK || out.ExitCode != 3 || !strings.Contains(out.Output, "broken") {
		t.Fatalf("failing run = %d %+v", code, out)
	}
	if code, out = runSkill(t, r, "probe-devops-only", `{"role":"lead_developer"}`); code != http.StatusForbidden || !strings.Contains(out.Error, "unauthorized") {
		t.Fatalf("role outside required_roles = %d %+v", code, out)
	}
	if code, _ = runSkill(t, r, "probe-denied", `{"role":"atdd_qa_engineer"}`); code != http.StatusForbidden {
		t.Fatalf("skill denied by the profile = %d", code)
	}
	if code, _ = runSkill(t, r, "probe-instructions", `{"role":"lead_developer"}`); code != http.StatusUnprocessableEntity {
		t.Fatalf("instruction-only skill = %d", code)
	}
	if code, _ = runSkill(t, r, "probe-run", `{"role":"intern"}`); code != http.StatusBadRequest {
		t.Fatalf("unknown role = %d", code)
	}
	if code, _ = runSkill(t, r, "probe-missing", `{"role":"lead_developer"}`); code != http.StatusNotFound {
		t.Fatalf("missing skill = %d", code)
	}
}
