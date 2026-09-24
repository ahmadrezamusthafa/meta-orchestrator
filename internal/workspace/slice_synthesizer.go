package workspace

import (
	"fmt"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
)

// SliceContextSynthesizer generates schema-compliant upstream artifact stubs when starting mid-process.
type SliceContextSynthesizer struct {
	artMgr *artifacts.ArtifactManager
}

// NewSliceContextSynthesizer creates a synthesizer.
func NewSliceContextSynthesizer(artMgr *artifacts.ArtifactManager) *SliceContextSynthesizer {
	return &SliceContextSynthesizer{artMgr: artMgr}
}

// SynthesizePrerequisites generates schema-valid PRD and Tech RFC stubs if missing.
func (s *SliceContextSynthesizer) SynthesizePrerequisites(taskID string, title string, description string) error {
	// 1. Synthesize PRD if missing
	_, _, err := s.artMgr.ReadArtifact(taskID, artifacts.ArtifactPRD)
	if err != nil {
		prdStub := fmt.Sprintf(`# Product Requirements: %s (Synthesized Mid-Process Slice)
## 1. Overview
%s

## Acceptance Criteria
- [x] Pre-existing specifications adopted for mid-process execution slice.
- [x] Initialized at %s.
`, title, description, time.Now().Format(time.RFC3339))

		_, err = s.artMgr.WriteArtifact(taskID, artifacts.ArtifactPRD, prdStub, "system_synthesizer")
		if err != nil {
			return fmt.Errorf("failed to synthesize PRD stub: %w", err)
		}
	}

	// 2. Synthesize Tech Doc RFC if missing
	_, _, err = s.artMgr.ReadArtifact(taskID, artifacts.ArtifactTechDocRFC)
	if err != nil {
		rfcStub := fmt.Sprintf(`# Technical Design: %s (Synthesized Mid-Process Slice)
## System Architecture
Pre-existing repository architecture adopted directly for implementation slice.

## API Contracts & Schemas
Referencing existing codebase contracts.
`, title)

		_, err = s.artMgr.WriteArtifact(taskID, artifacts.ArtifactTechDocRFC, rfcStub, "system_synthesizer")
		if err != nil {
			return fmt.Errorf("failed to synthesize RFC stub: %w", err)
		}
	}

	return nil
}
