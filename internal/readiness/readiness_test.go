package readiness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/packages"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

func TestEveryStepHasAnEmbeddedSkillNamedLikeIt(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Steps {
		if seen[s.ID] {
			t.Fatalf("duplicate step %s", s.ID)
		}
		seen[s.ID] = true
		raw, err := skillFS.ReadFile("skills/" + s.Skill + "/SKILL.md")
		if err != nil {
			t.Fatalf("%s: no embedded skill %s: %v", s.ID, s.Skill, err)
		}
		if !strings.Contains(string(raw), "\nname: "+s.Skill+"\n") {
			t.Fatalf("%s: SKILL.md frontmatter does not name %s", s.ID, s.Skill)
		}
		if len(s.Gate) == 0 || !strings.Contains(s.Instruction(), "- "+s.Gate[0]) {
			t.Fatalf("%s: instruction must carry the gate checklist", s.ID)
		}
		for _, g := range s.Gate {
			if strings.Contains(g, ";") {
				t.Fatalf("%s: gate item %q contains ';', the GateCriteria separator", s.ID, g)
			}
		}
	}
}

func TestInstallSkillsUpgradesUntouchedFilesAndKeepsEdits(t *testing.T) {
	dir := t.TempDir()
	if err := InstallSkills(dir); err != nil {
		t.Fatal(err)
	}
	audit := filepath.Join(dir, "launch-release-audit", "SKILL.md")
	api := filepath.Join(dir, "launch-customer-api", "SKILL.md")
	for _, p := range []string{audit, api} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("not installed: %v", err)
		}
	}

	// The operator edits one skill; an older build's copy of another is still untouched.
	if err := os.WriteFile(audit, []byte("my own audit"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := []byte("an older built-in version")
	if err := os.WriteFile(api, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := filepath.Join(dir, "launch-customer-api", installedManifest)
	if err := os.WriteFile(manifest, []byte(`{"SKILL.md":"`+digest(stale)+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallSkills(dir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(audit); string(got) != "my own audit" {
		t.Fatalf("operator edit was overwritten: %q", got)
	}
	want, _ := skillFS.ReadFile("skills/launch-customer-api/SKILL.md")
	if got, _ := os.ReadFile(api); string(got) != string(want) {
		t.Fatal("untouched built-in file was not upgraded")
	}
}

func TestEveryStepToolIsInstallableAndDocumented(t *testing.T) {
	man, err := packages.LoadManifest(filepath.Join("..", "..", "configs", "packages.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range Steps {
		raw, _ := skillFS.ReadFile("skills/" + s.Skill + "/SKILL.md")
		for _, n := range s.Tools {
			tool, ok := toolchain.Lookup(n.ID)
			if !ok || len(tool.Methods) == 0 {
				t.Errorf("%s: tool %s is not in the toolchain catalog with an install method", s.ID, n.ID)
			}
			if man.Get(n.ID) == nil {
				t.Errorf("%s: tool %s is not a Tool Hub package (configs/packages.json)", s.ID, n.ID)
			}
			if !strings.Contains(string(raw), "`"+tool.Binary+"`") {
				t.Errorf("%s: SKILL.md does not document %s", s.ID, tool.Binary)
			}
		}
	}
}
