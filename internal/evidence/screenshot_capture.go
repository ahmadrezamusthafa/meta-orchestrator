package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CapturedScreenshot records details of a saved milestone image.
type CapturedScreenshot struct {
	StepName   string    `json:"step_name"`
	FilePath   string    `json:"file_path"`
	SHA256Hash string    `json:"sha256_hash"`
	CapturedAt time.Time `json:"captured_at"`
	Width      int       `json:"width"`
	Height     int       `json:"height"`
}

// ScreenshotService manages viewport snapshot capture and indexing.
type ScreenshotService struct{}

// NewScreenshotService creates a screenshot service.
func NewScreenshotService() *ScreenshotService {
	return &ScreenshotService{}
}

// SaveMilestoneScreenshot records an image file with cryptographic SHA-256 fingerprint.
func (s *ScreenshotService) SaveMilestoneScreenshot(taskID string, stepName string, rawBytes []byte, outputDir string) (*CapturedScreenshot, error) {
	if outputDir == "" {
		outputDir = filepath.Join(".sdlc", "artifacts", taskID, "screenshots")
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create screenshot directory: %w", err)
	}

	fileName := fmt.Sprintf("step_%s.png", stepName)
	targetPath := filepath.Join(outputDir, fileName)

	if err := os.WriteFile(targetPath, rawBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write screenshot file: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(rawBytes)
	hashStr := hex.EncodeToString(hasher.Sum(nil))

	return &CapturedScreenshot{
		StepName:   stepName,
		FilePath:   targetPath,
		SHA256Hash: hashStr,
		CapturedAt: time.Now(),
		Width:      1280,
		Height:     720,
	}, nil
}
