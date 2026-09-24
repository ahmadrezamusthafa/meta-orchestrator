package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ws"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// VideoRecordConfig details capture dimensions and destination.
type VideoRecordConfig struct {
	Width       int
	Height      int
	OutputDir   string
	ProduceMP4  bool
}

// VideoRecorder manages execution video recording and WebSocket notification.
type VideoRecorder struct {
	mu         sync.Mutex
	transcoder *Transcoder
	wsHub      *ws.Hub
}

// NewVideoRecorder creates a recorder.
func NewVideoRecorder(wsHub *ws.Hub) *VideoRecorder {
	return &VideoRecorder{
		transcoder: NewTranscoder(),
		wsHub:      wsHub,
	}
}

// FinalizeRecording processes captured raw recording into final MP4 artifact.
func (r *VideoRecorder) FinalizeRecording(ctx context.Context, taskID string, rawVideoPath string, outputDir string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if outputDir == "" {
		outputDir = filepath.Join(".sdlc", "artifacts", taskID, "videos")
	}
	_ = os.MkdirAll(outputDir, 0755)

	finalMP4Path := filepath.Join(outputDir, "run_final.mp4")

	// Transcode to standard web MP4
	if err := r.transcoder.TranscodeToMP4(ctx, rawVideoPath, finalMP4Path); err != nil {
		return "", fmt.Errorf("failed to finalize video recording: %w", err)
	}

	// Emit WebSocket event for frontend Evidence Vault
	if r.wsHub != nil {
		videoURL := fmt.Sprintf("/api/v1/artifacts/%s/video", taskID)
		r.wsHub.BroadcastEvent(&types.OrchestratorEvent{
			Type:      types.EventType("evidence.video.ready"),
			TaskID:    taskID,
			Timestamp: time.Now(),
			Payload: map[string]string{
				"video_url":  videoURL,
				"local_path": finalMP4Path,
			},
		})
	}

	return finalMP4Path, nil
}
