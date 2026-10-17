package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// EventVideoReady is broadcast when a recorded video has been finalized as MP4.
const EventVideoReady types.EventType = "evidence.video.ready"

// Broadcaster publishes an event for a task (the API router's WebSocket broadcast).
type Broadcaster func(taskID string, typ types.EventType, payload interface{})

// VideoRecorder finalizes recorded videos as MP4, one at a time, and announces each one.
type VideoRecorder struct {
	mu         sync.Mutex
	transcoder *Transcoder
	broadcast  Broadcaster
}

// NewVideoRecorder creates a recorder; broadcast may be nil.
func NewVideoRecorder(broadcast Broadcaster) *VideoRecorder {
	return &VideoRecorder{transcoder: NewTranscoder(), broadcast: broadcast}
}

// FinalizeRecording transcodes rawVideoPath to an MP4 next to it, named after it (case-1.webm →
// case-1.mp4), and broadcasts EventVideoReady with the MP4's path relative to the evidence
// directory. Without ffmpeg it returns an error wrapping ErrNoFFmpeg and writes nothing.
func (r *VideoRecorder) FinalizeRecording(ctx context.Context, taskID, evidenceDir, rawVideoPath string) (string, error) {
	if strings.EqualFold(filepath.Ext(rawVideoPath), ".mp4") {
		return "", fmt.Errorf("%s is already an MP4", filepath.Base(rawVideoPath))
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := MP4Path(rawVideoPath)
	if err := r.transcoder.TranscodeToMP4(ctx, rawVideoPath, out); err != nil {
		return "", fmt.Errorf("failed to finalize %s: %w", filepath.Base(rawVideoPath), err)
	}
	if r.broadcast != nil {
		rel, err := filepath.Rel(evidenceDir, out)
		if err != nil {
			rel = filepath.Base(out)
		}
		rel = filepath.ToSlash(rel)
		r.broadcast(taskID, EventVideoReady, map[string]string{"video": rel, "source": filepath.Base(rawVideoPath),
			"video_url": fmt.Sprintf("/api/v1/artifacts/%s/evidence/%s", taskID, rel)})
	}
	return out, nil
}

// PendingMP4 lists the videos in the evidence directory (relative paths) that are not MP4 and have
// no MP4 sibling yet.
func PendingMP4(dir string) []string {
	var out []string
	for _, v := range listVideos(dir) {
		if strings.EqualFold(filepath.Ext(v), ".mp4") {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(MP4Path(v)))); err != nil {
			out = append(out, v)
		}
	}
	return out
}
