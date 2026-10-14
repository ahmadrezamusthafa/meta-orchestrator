package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSkillWritesInstructionsAndRecorder(t *testing.T) {
	dir := t.TempDir()
	if err := InstallSkill(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"SKILL.md", RecorderScript} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Fatalf("%s not installed: %v", f, err)
		}
	}
	inst := SkillInstructions()
	if strings.HasPrefix(inst, "---") || !strings.Contains(inst, "record-evidence.mjs") {
		t.Fatalf("instructions should be the SKILL.md body:\n%.200s", inst)
	}
	if err := InstallSkill(dir); err != nil { // idempotent
		t.Fatal(err)
	}
}

func TestCollectReadsVideosAndManifest(t *testing.T) {
	dir := t.TempDir()
	if r := Collect(dir); r.Status() != "missing" || len(r.Videos) != 0 || r.Manifest != nil {
		t.Fatalf("empty dir = %+v", r)
	}
	write := func(name, body string) {
		p := filepath.Join(dir, filepath.FromSlash(name))
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("TC-1.mp4", "v")
	write("sub/TC-2.webm", "v")
	write(".rec-TC-3/raw.webm", "v") // recorder scratch, ignored
	write("notes.txt", "x")
	if r := Collect(dir); r.Status() != "passed" || strings.Join(r.Videos, ",") != "TC-1.mp4,sub/TC-2.webm" {
		t.Fatalf("videos without manifest = %+v", r)
	}

	write(ManifestFile, `{"cases":[{"id":"TC-1","status":"passed","video":"TC-1.mp4"},
		{"id":"TC-2","status":"blocked","video":"../../etc/passwd"},{"id":"TC-4","status":"passed","video":"gone.mp4"}],
		"suites":[{"repository":"api","status":"failed"}]}`)
	r := Collect(dir)
	if r.Passed != 2 || r.Failed != 2 || r.Status() != "failed" {
		t.Fatalf("counts = %+v", r)
	}
	if r.Manifest.Cases[1].Video != "" {
		t.Fatalf("a path outside the evidence dir must be dropped, got %q", r.Manifest.Cases[1].Video)
	}
	if !r.Manifest.Cases[2].VideoMissing || r.Manifest.Cases[0].VideoMissing {
		t.Fatalf("video_missing = %+v", r.Manifest.Cases)
	}

	write(ManifestFile, "{not json")
	if r := Collect(dir); r.ManifestError == "" || r.Manifest != nil {
		t.Fatalf("invalid manifest = %+v", r)
	}
}
