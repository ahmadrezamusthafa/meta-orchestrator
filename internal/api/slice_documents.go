package api

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/workspace"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// sliceDocsKey lists the stages whose output came from documents uploaded with the task.
const sliceDocsKey = "slice_documents"

// ingestSliceDocuments turns the documents uploaded with a task that starts mid-pipeline into the
// outputs of the stages it skips, stored where those stages would have saved theirs.
func (r *Router) ingestSliceDocuments(t *types.Task, docs map[string]string, prd, taskPlan string) {
	all := map[string]string{}
	for name, content := range docs {
		all[name] = content
	}
	if prd != "" {
		all["PRD.md"] = prd
	}
	if taskPlan != "" {
		all["TASK_PLAN.md"] = taskPlan
	}
	if len(all) == 0 || t == nil {
		return
	}
	ingest, ignored := workspace.PlanSliceIngestion(all, stagePipeline, t.CurrentStageID)
	var stages, lines []string
	for _, d := range ingest {
		r.saveStageDocument(t.ID, d.Stage, "operator upload "+d.Name, d.Content)
		stages = append(stages, d.Stage)
		lines = append(lines, fmt.Sprintf("• %s → output of %s (%s)", d.Name, stageLabel(d.Stage), stageDocName(d.Stage)))
	}
	for _, why := range ignored {
		lines = append(lines, "• not used: "+why)
	}
	if len(stages) > 0 {
		r.mu.Lock()
		if live, ok := r.tasks[t.ID]; ok {
			if live.Metadata == nil {
				live.Metadata = map[string]string{}
			}
			live.Metadata[sliceDocsKey] = strings.Join(stages, ",")
		}
		r.mu.Unlock()
	}
	r.addEntry(t.ID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: "Uploaded documents:\n" + strings.Join(lines, "\n")})
}

// sliceDocsBrief points the agent at the documents that stand in for skipped stages.
func (r *Router) sliceDocsBrief(t *types.Task) string {
	stages := t.Metadata[sliceDocsKey]
	if stages == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nThis task started at a later stage. The operator supplied these documents in place of the stages it skipped; read them before you start:")
	for _, st := range strings.Split(stages, ",") {
		if p, err := r.resolveArtifactPath(t.ID, stageDocName(st)); err == nil && fileExists(p) {
			fmt.Fprintf(&b, "\n- %s: %s", stageLabel(st), p)
		}
	}
	return b.String()
}

// stageContext is the stage-specific part of the system prompt: the role's persona and the
// documents that replace skipped stages.
func (r *Router) stageContext(t *types.Task) string {
	return r.stagePersona(t) + r.sliceDocsBrief(t)
}
