package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ArtifactType represents standard SDLC artifact types.
type ArtifactType string

const (
	ArtifactPRD            ArtifactType = "PRD.md"
	ArtifactATDDSuite      ArtifactType = "ATDD_SUITE.md"
	ArtifactTechDocRFC     ArtifactType = "TECH_DOC_RFC.md"
	ArtifactTaskPlan       ArtifactType = "TASK_PLAN.md"
	ArtifactUATPreparation ArtifactType = "UAT_PREPARATION.md"
	ArtifactEvidence       ArtifactType = "EVIDENCE.md"
)

// ArtifactMetadata provides metadata about a saved artifact.
type ArtifactMetadata struct {
	Type        ArtifactType `json:"type"`
	TaskID      string       `json:"task_id"`
	Path        string       `json:"path"`
	SHA256Hash  string       `json:"sha256_hash"`
	SizeBytes   int64        `json:"size_bytes"`
	AuthorRole  string       `json:"author_role"`
	LastUpdated time.Time    `json:"last_updated"`
}

// ArtifactManager manages read, write, validation, and cryptographic hashing of artifacts.
type ArtifactManager struct {
	mu         sync.RWMutex
	baseDir    string // Typically /workspace/.sdlc/artifacts
	metadataDB map[string]*ArtifactMetadata
}

// NewArtifactManager creates a manager for artifacts under baseDir.
func NewArtifactManager(baseDir string) *ArtifactManager {
	return &ArtifactManager{
		baseDir:    baseDir,
		metadataDB: make(map[string]*ArtifactMetadata),
	}
}

// TaskArtifactDir returns the directory path for a task's artifacts.
func (m *ArtifactManager) TaskArtifactDir(taskID string) string {
	return filepath.Join(m.baseDir, taskID)
}

// WriteArtifact validates, hashes, and stores an artifact on disk.
func (m *ArtifactManager) WriteArtifact(taskID string, artType ArtifactType, content string, authorRole string) (*ArtifactMetadata, error) {
	if err := m.ValidateContent(artType, content); err != nil {
		return nil, fmt.Errorf("artifact validation failed: %w", err)
	}

	taskDir := m.TaskArtifactDir(taskID)
	if err := os.MkdirAll(taskDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create artifact dir: %w", err)
	}

	filePath := filepath.Join(taskDir, string(artType))
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write artifact file: %w", err)
	}

	// Compute SHA-256
	hasher := sha256.New()
	hasher.Write([]byte(content))
	hashStr := hex.EncodeToString(hasher.Sum(nil))

	meta := &ArtifactMetadata{
		Type:        artType,
		TaskID:      taskID,
		Path:        filePath,
		SHA256Hash:  hashStr,
		SizeBytes:   int64(len(content)),
		AuthorRole:  authorRole,
		LastUpdated: time.Now(),
	}

	m.mu.Lock()
	m.metadataDB[fmt.Sprintf("%s:%s", taskID, artType)] = meta
	m.mu.Unlock()

	return meta, nil
}

// ReadArtifact retrieves content and metadata for an artifact.
func (m *ArtifactManager) ReadArtifact(taskID string, artType ArtifactType) (string, *ArtifactMetadata, error) {
	filePath := filepath.Join(m.TaskArtifactDir(taskID), string(artType))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read artifact: %w", err)
	}

	m.mu.RLock()
	meta := m.metadataDB[fmt.Sprintf("%s:%s", taskID, artType)]
	m.mu.RUnlock()

	return string(data), meta, nil
}

// ValidateContent enforces minimum required headers for SDLC artifacts.
func (m *ArtifactManager) ValidateContent(artType ArtifactType, content string) error {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("artifact content cannot be empty")
	}

	switch artType {
	case ArtifactPRD:
		if !strings.Contains(trimmed, "# PRD") && !strings.Contains(trimmed, "# Product Requirements") {
			return fmt.Errorf("PRD.md must begin with a '# PRD' or '# Product Requirements' heading")
		}
	case ArtifactATDDSuite:
		if !strings.Contains(trimmed, "# Acceptance Test Suite") && !strings.Contains(trimmed, "Feature:") {
			return fmt.Errorf("ATDD_SUITE.md must contain '# Acceptance Test Suite' or Gherkin 'Feature:'")
		}
	case ArtifactTechDocRFC:
		if !strings.Contains(trimmed, "# Technical Design") && !strings.Contains(trimmed, "# Tech Doc") && !strings.Contains(trimmed, "# RFC") {
			return fmt.Errorf("TECH_DOC_RFC.md must include '# Technical Design', '# Tech Doc', or '# RFC'")
		}
	case ArtifactEvidence:
		if !strings.Contains(trimmed, "# Execution Evidence") && !strings.Contains(trimmed, "# EVIDENCE") {
			return fmt.Errorf("EVIDENCE.md must contain '# Execution Evidence' or '# EVIDENCE'")
		}
	}

	return nil
}
