package evidence

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// makeWebM writes a real one-second WebM with ffmpeg's test source, or skips without ffmpeg.
func makeWebM(t *testing.T, path string) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	out, err := exec.Command("ffmpeg", "-y", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc=duration=1:size=160x120:rate=10",
		"-c:v", "libvpx", "-b:v", "100k", path).CombinedOutput()
	if err != nil {
		t.Skipf("ffmpeg cannot encode a test WebM here: %v %s", err, out)
	}
}

func TestVideoRecorderFinalizationAndNotification(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "ABC-TC-001.webm")
	makeWebM(t, raw)

	var events []map[string]string
	rec := NewVideoRecorder(func(taskID string, typ types.EventType, payload interface{}) {
		if taskID != "task-vid-01" || typ != EventVideoReady {
			t.Errorf("event %s %s", taskID, typ)
		}
		events = append(events, payload.(map[string]string))
	})
	mp4, err := rec.FinalizeRecording(context.Background(), "task-vid-01", dir, raw)
	if err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if filepath.Base(mp4) != "ABC-TC-001.mp4" {
		t.Errorf("output must be named after the input, got %s", filepath.Base(mp4))
	}
	data, err := os.ReadFile(mp4)
	if err != nil || len(data) < 12 || !bytes.Equal(data[4:8], []byte("ftyp")) {
		t.Fatalf("not an MP4: %v", err)
	}
	if len(events) != 1 || events[0]["video"] != "ABC-TC-001.mp4" || events[0]["video_url"] != "/api/v1/artifacts/task-vid-01/evidence/ABC-TC-001.mp4" {
		t.Fatalf("events: %v", events)
	}
	if p := PendingMP4(dir); len(p) != 0 {
		t.Fatalf("nothing should be pending after transcoding: %v", p)
	}
	rep := Collect(dir)
	if len(rep.Videos) != 1 || rep.Videos[0] != "ABC-TC-001.mp4" {
		t.Fatalf("Collect must report the MP4 instead of the WebM: %v", rep.Videos)
	}
}

func TestTranscodeWithoutFFmpegWritesNothing(t *testing.T) {
	dir := t.TempDir()
	raw := filepath.Join(dir, "case.webm")
	if err := os.WriteFile(raw, []byte("webm"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	_, err := NewVideoRecorder(nil).FinalizeRecording(context.Background(), "T", dir, raw)
	if !errors.Is(err, ErrNoFFmpeg) {
		t.Fatalf("expected ErrNoFFmpeg, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "case.mp4")); !os.IsNotExist(err) {
		t.Fatal("no MP4 may be written without ffmpeg")
	}
	if p := PendingMP4(dir); len(p) != 1 || p[0] != "case.webm" {
		t.Fatalf("pending: %v", p)
	}
}

func TestCollectPrefersMP4InManifest(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"A.webm", "A.mp4", "B.webm"} {
		_ = os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"cases":[{"id":"A","status":"passed","video":"A.webm"},{"id":"B","status":"passed","video":"B.webm"}]}`), 0o644)
	rep := Collect(dir)
	if rep.Manifest.Cases[0].Video != "A.mp4" || rep.Manifest.Cases[1].Video != "B.webm" || rep.Manifest.Cases[0].VideoMissing {
		t.Fatalf("cases: %+v", rep.Manifest.Cases)
	}
	if len(rep.Videos) != 2 {
		t.Fatalf("videos: %v", rep.Videos)
	}
}
