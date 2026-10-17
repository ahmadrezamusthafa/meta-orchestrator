package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegistriesReportLoadedSkillsHooksMethodsAndProfiles(t *testing.T) {
	root := profileRoot(t)
	writeSkill(t, root, "probe-registry-skill", "A real project skill", "Do it.")
	files := map[string]string{
		".sdlc/hooks.json": `[
  {"id": "lint", "event": "pre-commit", "driver": "shell", "target": "make lint", "is_blocking": true, "timeout_sec": 30},
  {"id": "notify", "event": "on-failure", "driver": "webhook", "target": "https://hooks.example.invalid/services/SECRETPATH?token=abc", "is_blocking": false}
]`,
		".sdlc/hooks/pre-stage.sh": "#!/bin/sh\nexit 0\n",
		"configs/methods.json":     `[{"id": "react", "name": "ReAct", "execution_pattern": "single_react", "allowed_roles": ["lead_developer"], "max_consecutive_iterations": 10}]`,
	}
	for path, body := range files {
		full := filepath.Join(root, path)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := NewRouter(RouterConfig{RootDir: root})

	w := do(r, http.MethodGet, "/api/v1/registries", "")
	if w.Code != http.StatusOK {
		t.Fatalf("registries = %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "SECRETPATH") || strings.Contains(w.Body.String(), "token=abc") {
		t.Fatal("webhook URL path/query must not be exposed")
	}
	var got RegistryDTO
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}

	skills := map[string]SkillItemDTO{}
	for _, s := range got.Skills {
		skills[s.Name] = s
	}
	if s, ok := skills["probe-registry-skill"]; !ok || s.Source != "claude" || !s.Enabled {
		t.Fatalf("project skill missing or wrong: %+v", s)
	}
	if _, fake := skills["claude_atdd_generator"]; fake {
		t.Fatal("hard-coded sample skills are still listed")
	}

	if len(got.Hooks) != 3 {
		t.Fatalf("hooks = %+v", got.Hooks)
	}
	if h := got.Hooks[0]; h.ID != "lint" || h.Type != "SHELL" || h.Policy != "BLOCK" || h.Command != "make lint" {
		t.Fatalf("hook 0 = %+v", h)
	}
	if h := got.Hooks[1]; h.Type != "WEBHOOK" || h.Policy != "WARN" || h.Command != "https://hooks.example.invalid/…" {
		t.Fatalf("hook 1 = %+v", h)
	}
	if h := got.Hooks[2]; h.Event != "pre-stage" || h.Policy != "BLOCK" || h.Source != filepath.Join(".sdlc", "hooks", "pre-stage.sh") {
		t.Fatalf("convention hook = %+v", h)
	}

	if len(got.Methods) != 1 || got.Methods[0].ID != "react" || got.Methods[0].MaxConsecutiveIterations != 10 {
		t.Fatalf("methods = %+v", got.Methods)
	}
	if len(got.Profiles) != 2 || got.Profiles[0].ID != "atdd_qa_engineer" ||
		strings.Join(got.Profiles[0].Stages, ",") != "atdd_creation,e2e_validation,uat_verification" {
		t.Fatalf("profiles = %+v", got.Profiles)
	}
	if len(got.Problems) != 0 {
		t.Fatalf("problems = %v", got.Problems)
	}
}

func TestRegistriesReportUnreadableHooksFile(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, ".sdlc"), 0o755)
	_ = os.WriteFile(filepath.Join(root, ".sdlc", "hooks.json"), []byte("{not json"), 0o644)
	r := NewRouter(RouterConfig{RootDir: root})
	var got RegistryDTO
	_ = json.Unmarshal(do(r, http.MethodGet, "/api/v1/registries", "").Body.Bytes(), &got)
	if len(got.Problems) != 1 || !strings.Contains(got.Problems[0], ".sdlc/hooks.json") || len(got.Hooks) != 0 || len(got.Methods) != 0 {
		t.Fatalf("got %+v", got)
	}
}
