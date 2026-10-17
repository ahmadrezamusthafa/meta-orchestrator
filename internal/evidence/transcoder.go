package evidence

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ErrNoFFmpeg is returned when ffmpeg is not installed: nothing is transcoded and no MP4 is written.
var ErrNoFFmpeg = errors.New("ffmpeg is not installed, so the video was not transcoded to MP4")

// Transcoder converts raw video captures into streaming-friendly MP4 files with ffmpeg.
type Transcoder struct{}

// NewTranscoder creates a transcoder.
func NewTranscoder() *Transcoder {
	return &Transcoder{}
}

// Available reports whether ffmpeg is installed.
func (t *Transcoder) Available() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// TranscodeToMP4 converts the input video to H.264 MP4 with faststart. The output is written to a
// hidden temporary file first and renamed into place, so a partial MP4 is never left behind.
func (t *Transcoder) TranscodeToMP4(ctx context.Context, inputPath string, outputPath string) error {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ErrNoFFmpeg
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(outputPath), "."+filepath.Base(outputPath)+".part")
	defer os.Remove(tmp)
	cmd := exec.CommandContext(ctx, bin, "-y", "-loglevel", "error", "-i", inputPath,
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", "-f", "mp4", tmp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg transcoding failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return os.Rename(tmp, outputPath)
}

// MP4Path is the MP4 that a video transcodes to: the same name with the .mp4 extension.
func MP4Path(videoPath string) string {
	return strings.TrimSuffix(videoPath, filepath.Ext(videoPath)) + ".mp4"
}
