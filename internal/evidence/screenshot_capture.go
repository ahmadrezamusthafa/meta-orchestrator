package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg" // register decoders for image.DecodeConfig
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CapturedScreenshot is one screenshot in an evidence directory, fingerprinted for the manifest.
type CapturedScreenshot struct {
	FilePath   string    `json:"file_path"` // relative to the evidence directory
	SHA256Hash string    `json:"sha256_hash"`
	SizeBytes  int64     `json:"size_bytes"`
	CapturedAt time.Time `json:"captured_at"` // the file's modification time
	// Width and Height are read from the image header; 0 when the format cannot be decoded (webp).
	Width  int `json:"width,omitempty"`
	Height int `json:"height,omitempty"`
}

var screenshotExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

// IsScreenshot reports whether name is an image file.
func IsScreenshot(name string) bool { return screenshotExts[strings.ToLower(filepath.Ext(name))] }

// ScreenshotService indexes the screenshots an evidence recorder saved.
type ScreenshotService struct{}

// NewScreenshotService creates a screenshot service.
func NewScreenshotService() *ScreenshotService {
	return &ScreenshotService{}
}

// Describe fingerprints one screenshot: its SHA-256, size and real dimensions. rel is the path
// recorded in the result.
func (s *ScreenshotService) Describe(path, rel string) (*CapturedScreenshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	shot := &CapturedScreenshot{FilePath: rel, SHA256Hash: hex.EncodeToString(h.Sum(nil)), SizeBytes: st.Size(), CapturedAt: st.ModTime()}
	if _, err := f.Seek(0, io.SeekStart); err == nil {
		if cfg, _, err := image.DecodeConfig(f); err == nil {
			shot.Width, shot.Height = cfg.Width, cfg.Height
		}
	}
	return shot, nil
}
