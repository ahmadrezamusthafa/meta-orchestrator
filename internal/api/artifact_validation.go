package api

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/artifacts"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// maxDocIssuesShown bounds the issues listed in the console note.
const maxDocIssuesShown = 8

// checkStageDocument checks a saved stage document against what its stage brief asks for. The
// check is advisory: issues become a ⚠ console note and a short result in the task's metadata
// (doc_check_<stage>), and the stage still goes to review as usual.
func (r *Router) checkStageDocument(taskID, stage, content string) {
	errs, checked := artifacts.NewArtifactValidator().ValidateStage(stage, content)
	if !checked {
		return
	}
	result := "ok"
	if len(errs) > 0 {
		result = fmt.Sprintf("%d issue%s", len(errs), plural(len(errs)))
	}
	r.mu.Lock()
	if t, ok := r.tasks[taskID]; ok {
		if t.Metadata == nil {
			t.Metadata = map[string]string{}
		}
		t.Metadata["doc_check_"+stage] = result
	}
	r.mu.Unlock()
	if len(errs) == 0 {
		return
	}
	shown := errs
	if len(shown) > maxDocIssuesShown {
		shown = shown[:maxDocIssuesShown]
	}
	var b strings.Builder
	fmt.Fprintf(&b, "⚠ The %s document may be incomplete (advisory, review it before approving):\n", stage)
	b.WriteString(strings.TrimRight(artifacts.FormatValidationErrors(shown), "\n"))
	if more := len(errs) - len(shown); more > 0 {
		fmt.Fprintf(&b, "\n- … and %d more", more)
	}
	r.addEntry(taskID, types.ConsoleEntry{Kind: types.ConsoleKindSystem, Content: b.String()})
}
