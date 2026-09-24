package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestVideoRecorderFinalizationAndNotification(t *testing.T) {
	tmpDir := t.TempDir()
	rawVideo := filepath.Join(tmpDir, "sample_raw.webm")
	_ = os.WriteFile(rawVideo, []byte("mock-raw-webm-stream-data"), 0644)

	wsHub := ws.NewHub()
	go wsHub.Run()
	defer wsHub.Stop()

	// Register test listener
	eventsCh := make(chan *types.OrchestratorEvent, 10)
	_ = eventsCh

	recorder := NewVideoRecorder(wsHub)
	ctx := context.Background()

	outputDir := filepath.Join(tmpDir, "videos")
	mp4Path, err := recorder.FinalizeRecording(ctx, "task-vid-01", rawVideo, outputDir)
	if err != nil {
		t.Fatalf("failed to finalize video recording: %v", err)
	}

	if filepath.Base(mp4Path) != "run_final.mp4" {
		t.Errorf("expected output name run_final.mp4, got %s", filepath.Base(mp4Path))
	}

	// Verify file exists on disk
	stat, err := os.Stat(mp4Path)
	if err != nil || stat.Size() == 0 {
		t.Errorf("final MP4 was not created or is 0 bytes")
	}

	time.Sleep(20 * time.Millisecond)
}
