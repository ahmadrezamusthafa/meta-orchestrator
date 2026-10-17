package techdoc

import (
	"strings"
	"testing"
	"time"
)

func TestRenderFramesTheAgentContentVerbatim(t *testing.T) {
	content := "## Approach\nAdd a refund service.\n\n## Risks\nDouble refunds."
	at := time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC)
	doc := Render(Meta{TaskID: "TASK-7", Title: "Refunds", Stage: "techdoc_rfc", Model: "claude-x", Repos: []string{"backend-core"}, Generated: at}, content)
	want := "# Technical design: Refunds\n\n> Task TASK-7 · stage techdoc_rfc · repositories backend-core · written by claude-x · 2026-10-17 09:30\n\n" + content
	if doc != want {
		t.Fatalf("doc:\n%s", doc)
	}
	for _, fabricated := range []string{"mermaid", "sequenceDiagram", "```http"} {
		if strings.Contains(doc, fabricated) {
			t.Fatalf("Render must not add content: %s", fabricated)
		}
	}

	// The agent's own title is kept; only the metadata line is added under it.
	doc = Render(Meta{TaskID: "TASK-7", Stage: "task_breakdown"}, "# Refund plan\n1. Add migration")
	if doc != "# Refund plan\n\n> Task TASK-7 · stage task_breakdown\n\n1. Add migration" {
		t.Fatalf("doc:\n%s", doc)
	}
	if !Frames("techdoc_rfc") || !Frames("task_breakdown") || Frames("prd_discovery") {
		t.Fatal("only the technical design and breakdown are framed")
	}
}

func TestSectionsAndSteps(t *testing.T) {
	if m := MissingSections("**Approach:** x\n## Components\n| Risk | Mitigation |", RFCSections); strings.Join(m, ",") != "Data / API changes,Alternatives considered" {
		t.Fatalf("missing = %v", m)
	}
	if n := CountSteps("1. one\n2. two\n- [ ] three\n| 4 | four |\n### Task 5\nprose 6. not a step"); n != 5 {
		t.Fatalf("steps = %d", n)
	}
}
