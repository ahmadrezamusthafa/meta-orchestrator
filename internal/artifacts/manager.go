package artifacts

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	AuthorRole  string       `json:"author_role,omitempty"` // only known to the writer; not stored
	LastUpdated time.Time    `json:"last_updated"`
}

// ArtifactManager is the registry of task artifacts: everything lives on disk under
// <root>/.sdlc/artifacts/<task>, and metadata is always read from the files themselves.
type ArtifactManager struct {
	baseDir string
}

// NewArtifactManager creates a manager for artifacts under baseDir (<root>/.sdlc/artifacts in the
// daemon).
func NewArtifactManager(baseDir string) *ArtifactManager {
	return &ArtifactManager{baseDir: baseDir}
}

// TaskArtifactDir returns the directory path for a task's artifacts.
func (m *ArtifactManager) TaskArtifactDir(taskID string) string {
	return filepath.Join(m.baseDir, taskID)
}

// Entry describes one file in a task's artifact directory.
type Entry struct {
	Path       string    `json:"path"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"` // "stage_output", "document", "media", "other"
	StageID    string    `json:"stage_id,omitempty"`
	Size       int64     `json:"size"`
	ModifiedAt time.Time `json:"modified_at"`
}

var mediaExts = map[string]bool{".mp4": true, ".webm": true, ".mov": true, ".png": true, ".jpg": true, ".jpeg": true, ".webp": true, ".svg": true}

// List returns every non-hidden file in the task's artifact directory, sorted by path.
func (m *ArtifactManager) List(taskID string) []Entry {
	base := m.TaskArtifactDir(taskID)
	entries := []Entry{}
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && path != base {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(base, path)
		rel = filepath.ToSlash(rel)
		e := Entry{Path: rel, Name: d.Name(), Size: info.Size(), ModifiedAt: info.ModTime(), Kind: "other"}
		ext := strings.ToLower(filepath.Ext(path))
		switch {
		case strings.HasPrefix(rel, "stages/") && ext == ".md":
			e.Kind = "stage_output"
			stage := strings.TrimSuffix(d.Name(), ".md")
			if i := strings.Index(stage, "-"); i > 0 {
				stage = stage[i+1:]
			}
			e.StageID = stage
		case ext == ".md" || ext == ".txt" || ext == ".json" || ext == ".yaml" || ext == ".yml":
			e.Kind = "document"
		case mediaExts[ext]:
			e.Kind = "media"
		}
		entries = append(entries, e)
		return nil
	})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
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

	sum := sha256.Sum256([]byte(content))
	return &ArtifactMetadata{
		Type:        artType,
		TaskID:      taskID,
		Path:        filePath,
		SHA256Hash:  hex.EncodeToString(sum[:]),
		SizeBytes:   int64(len(content)),
		AuthorRole:  authorRole,
		LastUpdated: time.Now(),
	}, nil
}

// ReadArtifact retrieves content and metadata (hash, size, modification time) for an artifact.
func (m *ArtifactManager) ReadArtifact(taskID string, artType ArtifactType) (string, *ArtifactMetadata, error) {
	filePath := filepath.Join(m.TaskArtifactDir(taskID), string(artType))
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read artifact: %w", err)
	}
	st, err := os.Stat(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("failed to read artifact: %w", err)
	}
	sum := sha256.Sum256(data)
	return string(data), &ArtifactMetadata{Type: artType, TaskID: taskID, Path: filePath, SHA256Hash: hex.EncodeToString(sum[:]),
		SizeBytes: int64(len(data)), LastUpdated: st.ModTime()}, nil
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
