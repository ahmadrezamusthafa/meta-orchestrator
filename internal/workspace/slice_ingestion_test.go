package workspace

import (
	"strings"
	"testing"
)

var pipeline = []string{"prd_discovery", "atdd_creation", "techdoc_rfc", "task_breakdown", "task_implementation", "e2e_validation"}

func TestPlanSliceIngestionMapsDocsToSkippedStages(t *testing.T) {
	docs := map[string]string{
		"PRD.md":       "# PRD",
		"Tech_Doc.md":  "# RFC",
		"TASK_PLAN.md": "# Plan",
		"notes.md":     "# Notes",
		"RFC.md":       "# Second RFC",
		"empty/PRD.md": " ",
	}
	ingest, ignored := PlanSliceIngestion(docs, pipeline, "task_implementation")
	var got []string
	for _, d := range ingest {
		got = append(got, d.Stage+"="+d.Name)
	}
	if strings.Join(got, ",") != "prd_discovery=PRD.md,techdoc_rfc=RFC.md,task_breakdown=TASK_PLAN.md" {
		t.Fatalf("ingest = %v", got)
	}
	want := []string{"Tech_Doc.md: RFC.md already provides techdoc_rfc", "empty/PRD.md: empty", "notes.md: not a PRD.md, TECH_DOC.md or TASK_PLAN.md"}
	if strings.Join(ignored, "|") != strings.Join(want, "|") {
		t.Fatalf("ignored = %q", ignored)
	}
}

func TestPlanSliceIngestionLeavesStagesTheTaskRuns(t *testing.T) {
	ingest, ignored := PlanSliceIngestion(map[string]string{"PRD.md": "# PRD", "TASK_PLAN.md": "# Plan"}, pipeline, "techdoc_rfc")
	if len(ingest) != 1 || ingest[0].Stage != "prd_discovery" {
		t.Fatalf("ingest = %+v", ingest)
	}
	if len(ignored) != 1 || ignored[0] != "TASK_PLAN.md: the task runs task_breakdown itself" {
		t.Fatalf("ignored = %q", ignored)
	}
}
