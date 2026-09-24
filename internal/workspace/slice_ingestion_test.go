package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/fsm"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

func TestSliceIngestionAndUpstreamSynthesis(t *testing.T) {
	tmpDir := t.TempDir()
	artMgr := artifacts.NewArtifactManager(tmpDir)
	ingestion := NewSliceIngestionEngine(artMgr)

	reg := fsm.NewWorkflowRegistry()
	general, _ := reg.Get("general-ai-sdlc")

	task := &types.Task{
		ID:          "task-slice-01",
		Title:       "Payment Gateway Implementation",
		Description: "Directly implement Stripe payment integration",
		CreatedAt:   time.Now(),
	}

	ws := &Workspace{
		TaskID:    task.ID,
		BasePath:  tmpDir,
		RepoPaths: map[string]string{"backend": tmpDir},
	}

	// Slice starting at Stage 6 (IMPLEMENTATION_GREEN) and halting at Stage 7 (E2E_AUTOMATION)
	slice := &types.StageSlice{
		StartStageID: "IMPLEMENTATION_GREEN",
		HaltStageID:  "E2E_AUTOMATION",
		ProduceVideo: true,
	}

	input := &ExternalSliceInput{
		ExternalTaskPlan: "## Tasks\n- TASK-1: Wire Stripe API",
		ProduceVideo:     true,
	}

	ctx := context.Background()
	plan, err := ingestion.IngestAndHydrate(ctx, ws, task, general, slice, input)
	if err != nil {
		t.Fatalf("ingestion failed: %v", err)
	}

	if plan.EffectiveStages[0].ID != "IMPLEMENTATION_GREEN" {
		t.Errorf("expected first stage to be IMPLEMENTATION_GREEN, got %s", plan.EffectiveStages[0].ID)
	}
	if task.CurrentStageID != "IMPLEMENTATION_GREEN" {
		t.Errorf("expected task stage to be IMPLEMENTATION_GREEN, got %s", task.CurrentStageID)
	}

	// Verify synthesized upstream PRD and Tech Doc artifacts exist
	prdContent, _, err := artMgr.ReadArtifact(task.ID, artifacts.ArtifactPRD)
	if err != nil || prdContent == "" {
		t.Errorf("expected synthesized PRD artifact to exist")
	}

	rfcContent, _, err := artMgr.ReadArtifact(task.ID, artifacts.ArtifactTechDocRFC)
	if err != nil || rfcContent == "" {
		t.Errorf("expected synthesized RFC artifact to exist")
	}
}
