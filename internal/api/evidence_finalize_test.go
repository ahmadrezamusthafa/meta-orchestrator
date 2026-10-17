package api

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/evidence"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// TestEvidenceTurnTranscodesWebMAndSignsTheManifest drives the implementation stage: the agent
// records a real WebM, and when the turn ends the orchestrator makes the MP4, the Evidence API
// serves the MP4, and the signed manifest verifies until a file is changed.
func TestEvidenceTurnTranscodesWebMAndSignsTheManifest(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	stub := &evidenceStub{}
	r, root := evidenceRouter(t, stub)
	t.Setenv(evidence.SigningKeyEnv, "")
	dir := r.evidenceDir("TASK-V1")
	stub.onRun = func() {
		_ = os.MkdirAll(dir, 0o755)
		out, err := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=10",
			"-c:v", "libvpx", "-b:v", "100k", filepath.Join(dir, "TC-1.webm")).CombinedOutput()
		if err != nil {
			t.Errorf("make webm: %v %s", err, out)
		}
		raw, _ := json.Marshal(evidence.Manifest{Cases: []evidence.Case{{ID: "TC-1", Status: "passed", Video: "TC-1.webm"}}})
		_ = os.WriteFile(filepath.Join(dir, evidence.ManifestFile), raw, 0o644)
	}
	r.mu.Lock()
	r.tasks["TASK-V1"] = &types.Task{ID: "TASK-V1", Title: "Refund flow", CurrentStageID: "task_implementation", State: types.TaskStatePending,
		AssignedRepos: []string{"backend-core"}, Metadata: map[string]string{"complexity": "LOW"}}
	r.mu.Unlock()
	if w := do(r, "POST", "/api/v1/tasks/TASK-V1/execute", ""); w.Code != 200 {
		t.Fatalf("execute → %d %s", w.Code, w.Body.String())
	}
	waitState(t, r, "TASK-V1", types.TaskStateWaitingGateApproval)
	r.evidenceJobs.Wait()

	if _, err := os.Stat(filepath.Join(dir, "TC-1.mp4")); err != nil {
		t.Fatalf("no MP4 transcode: %v", err)
	}
	if st, err := os.Stat(filepath.Join(root, evidence.SigningKeyFile)); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("signing key: %v %v", st, err)
	}
	log := consoleText(r, "TASK-V1")
	for _, want := range []string{"Transcoded 1 WebM video to MP4: TC-1.mp4", "Evidence manifest signed", "1 passed, 0 failed"} {
		if !strings.Contains(log, want) {
			t.Fatalf("console is missing %q:\n%s", want, log)
		}
	}
	w := do(r, "GET", "/api/v1/tasks/TASK-V1/evidence", "")
	if !strings.Contains(w.Body.String(), `"video":"TC-1.mp4"`) {
		t.Fatalf("the Evidence API must point the case at the MP4: %s", w.Body.String())
	}

	var got struct{ Verify evidence.VerifyResult }
	w = do(r, "GET", "/api/v1/tasks/TASK-V1/evidence/verify", "")
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if w.Code != 200 || !got.Verify.Valid || got.Verify.RecordedFiles < 3 {
		t.Fatalf("verify → %d %s", w.Code, w.Body.String())
	}
	_ = os.WriteFile(filepath.Join(dir, "TC-1.mp4"), []byte("replaced"), 0o644)
	w = do(r, "GET", "/api/v1/tasks/TASK-V1/evidence/verify", "")
	got.Verify = evidence.VerifyResult{}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Verify.Valid || strings.Join(got.Verify.Changed, ",") != "TC-1.mp4" {
		t.Fatalf("a replaced video must be reported: %s", w.Body.String())
	}
	if w := do(r, "GET", "/api/v1/tasks/NOPE/evidence/verify", ""); w.Code != 404 {
		t.Fatalf("unknown task → %d", w.Code)
	}
}

func TestEvidenceWithoutFFmpegKeepsWebMAndSaysSo(t *testing.T) {
	r, _ := evidenceRouter(t, &evidenceStub{})
	t.Setenv(evidence.SigningKeyEnv, "test-signing-key")
	dir := r.evidenceDir("TASK-V2")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, "TC-1.webm"), []byte("webm"), 0o644)
	r.mu.Lock()
	r.tasks["TASK-V2"] = &types.Task{ID: "TASK-V2", CurrentStageID: "task_implementation", Metadata: map[string]string{}}
	r.mu.Unlock()
	t.Setenv("PATH", t.TempDir())

	r.reportEvidence("TASK-V2", "turn-1", &evidencePrep{Dir: dir})
	r.evidenceJobs.Wait()

	if _, err := os.Stat(filepath.Join(dir, "TC-1.mp4")); !os.IsNotExist(err) {
		t.Fatal("no MP4 may be made without ffmpeg")
	}
	log := consoleText(r, "TASK-V2")
	if !strings.Contains(log, "ffmpeg is not installed, so no MP4 copies were made") || !strings.Contains(log, "Evidence manifest signed") {
		t.Fatalf("console:\n%s", log)
	}
	if r.taskSnapshot("TASK-V2").Metadata["evidence_manifest"] == "" {
		t.Fatal("the manifest time is stored on the task")
	}
}
