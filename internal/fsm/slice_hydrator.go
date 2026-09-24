package fsm

import (
	"fmt"

	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// SliceHydrator validates and configures partial execution windows for workflows.
type SliceHydrator struct{}

// NewSliceHydrator creates a new hydrator instance.
func NewSliceHydrator() *SliceHydrator {
	return &SliceHydrator{}
}

// SliceExecutionPlan encapsulates computed boundary for a sliced task execution.
type SliceExecutionPlan struct {
	WorkflowID       string
	OriginalStages   []types.WorkflowStage
	EffectiveStages  []types.WorkflowStage
	StartIndex       int
	HaltIndex        int
	RequiresVideo    bool
	MissingArtifacts []string
}

// HydrateSlice computes the valid execution bounds for a given workflow and slice request.
func (h *SliceHydrator) HydrateSlice(def *types.WorkflowDefinition, slice *types.StageSlice, existingArtifacts []string) (*SliceExecutionPlan, error) {
	if def == nil {
		return nil, fmt.Errorf("workflow definition cannot be nil")
	}

	if slice == nil || (slice.StartStageID == "" && slice.HaltStageID == "") {
		// Full workflow execution
		return &SliceExecutionPlan{
			WorkflowID:      def.ID,
			OriginalStages:  def.Stages,
			EffectiveStages: def.Stages,
			StartIndex:      0,
			HaltIndex:       len(def.Stages) - 1,
			RequiresVideo:   false,
		}, nil
	}

	startIdx := -1
	haltIdx := -1

	// Locate start stage
	if slice.StartStageID != "" {
		for i, s := range def.Stages {
			if s.ID == slice.StartStageID {
				startIdx = i
				break
			}
		}
		if startIdx == -1 {
			return nil, fmt.Errorf("start stage '%s' not found in workflow '%s'", slice.StartStageID, def.ID)
		}
	} else {
		startIdx = 0
	}

	// Locate halt stage
	if slice.HaltStageID != "" {
		for i, s := range def.Stages {
			if s.ID == slice.HaltStageID {
				haltIdx = i
				break
			}
		}
		if haltIdx == -1 {
			return nil, fmt.Errorf("halt stage '%s' not found in workflow '%s'", slice.HaltStageID, def.ID)
		}
	} else {
		haltIdx = len(def.Stages) - 1
	}

	if startIdx > haltIdx {
		return nil, fmt.Errorf("invalid slice range: start stage '%s' (index %d) is after halt stage '%s' (index %d)",
			slice.StartStageID, startIdx, slice.HaltStageID, haltIdx)
	}

	// Verify required prerequisites if starting in the middle
	existingMap := make(map[string]bool)
	for _, art := range existingArtifacts {
		existingMap[art] = true
	}

	var missingPrereqs []string
	if startIdx > 0 {
		for i := 0; i < startIdx; i++ {
			for _, art := range def.Stages[i].RequiredArtifacts {
				if !existingMap[art] {
					missingPrereqs = append(missingPrereqs, art)
				}
			}
		}
	}

	effective := def.Stages[startIdx : haltIdx+1]

	return &SliceExecutionPlan{
		WorkflowID:       def.ID,
		OriginalStages:   def.Stages,
		EffectiveStages:  effective,
		StartIndex:       startIdx,
		HaltIndex:        haltIdx,
		RequiresVideo:    slice.ProduceVideo,
		MissingArtifacts: missingPrereqs,
	}, nil
}
