package artifacts

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Realistic stage outputs, written the way the stage briefs ask for them.

const goodPRD = `I read the billing repositories and the ticket. Here is the clarified requirement.

## Goals
- Finance can refund a paid invoice fully or partially from the admin.

## Scope and non-goals
**In scope:** card and bank-transfer invoices.
**Non-goals:** refunds of invoices older than 180 days.

## Acceptance criteria
1. Given a paid invoice, when finance refunds 50%, then the invoice shows "Partially refunded".
2. Given an unpaid invoice, the Refund button is hidden.

## Assumptions
- The payment gateway supports partial refunds.

## Open questions
- Should refunds notify the customer by email?

## Summary
Requirement clarified; two open questions for the PM.`

const goodPRDLabels = `**Goals:** let finance refund a paid invoice.
**Scope:** admin refunds only. **Out of scope:** customer self-service.
**Acceptance criteria:**
- AC-1 refund a paid invoice
**Assumptions:** gateway supports partial refunds.
**Open questions:** none yet.`

const goodATDD = "Cases in Given/When/Then form:\n\n```atdd-cases\n" +
	`[{"id": "BIL-TC-001", "requirement": "US-1", "priority": "P1", "test_level": "SIT", "title": "Finance refunds half of a paid invoice",
  "platform": "WEB", "repository": "billing-frontend", "uat": true, "sanity": false,
  "preconditions": ["I am signed in as Finance."], "steps": ["1. Open Invoices", "2. Click Refund"], "expected_results": ["1. The invoice shows Partially refunded"]},
 {"id": "BIL-TC-002", "priority": "P0", "test_level": "SUT", "title": "Refund API rejects an unpaid invoice", "platform": "API",
  "steps": ["POST /refunds for an unpaid invoice"], "expected_results": ["422 with invoice_not_paid"]}]` + "\n```\n\n## Summary\nTwo cases."

const goodRFC = `## Approach
Add a refund service behind the existing invoice API.

## Components
- RefundService (backend-core)

## Data / API changes
- New table refunds; POST /api/v1/invoices/{id}/refunds

## Risks
- Double refunds under retries: guarded by an idempotency key.

## Alternatives considered
- Refunding through the gateway dashboard only: rejected, no audit trail.`

const goodBreakdown = `1. Add the refunds migration — repository backend-core, files db/migrate/20261017_refunds.rb; verify with rails db:migrate and the model spec.
2. Add RefundService — backend-core, app/services/refund_service.rb; verified by spec/services/refund_service_spec.rb.
3. Add the Refund button — billing-frontend, src/views/Invoice.vue; verify with the component test.`

func TestValidatorAcceptsRealisticStageOutputs(t *testing.T) {
	v := NewArtifactValidator()
	for name, c := range map[string]struct{ stage, content string }{
		"prd headings": {"prd_discovery", goodPRD}, "prd labels": {"prd_discovery", goodPRDLabels},
		"atdd": {"atdd_creation", goodATDD}, "rfc": {"techdoc_rfc", goodRFC}, "breakdown": {"task_breakdown", goodBreakdown},
	} {
		errs, checked := v.ValidateStage(c.stage, c.content)
		if !checked || len(errs) != 0 {
			t.Errorf("%s: want zero warnings, got %v", name, errs)
		}
	}
	if _, checked := v.ValidateStage("repo_discovery", "anything"); checked {
		t.Error("stages without a brief-derived check are not checked")
	}
}

func TestValidatorNamesWhatIsMissing(t *testing.T) {
	v := NewArtifactValidator()
	errs := v.ValidatePRD("## Goals\n- refund invoices\n## Summary\ndone")
	got := FormatValidationErrors(errs)
	for _, want := range []string{"Non-goals", "Acceptance criteria", "Assumptions", "Open questions"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "no Goals") {
		t.Errorf("Goals is present: %s", got)
	}

	if errs := v.ValidateATDD("Given a paid invoice When refunded Then …"); len(errs) != 1 || !strings.Contains(errs[0].Message, "no ```atdd-cases block") {
		t.Errorf("atdd without block: %v", errs)
	}
	bad := "```atdd-cases\n[{\"id\": \"X-1\", \"priority\": \"high\", \"test_level\": \"E2E\", \"platform\": \"desktop\"}]\n```"
	got = FormatValidationErrors(v.ValidateATDD(bad))
	for _, want := range []string{"[X-1]", "no title", "no steps", "no expected results", `priority "high"`, `test_level "E2E"`, `platform "desktop"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}

	if got := FormatValidationErrors(v.ValidateTechDoc("## Approach\nAdd a service.")); !strings.Contains(got, "Risks") || !strings.Contains(got, "Alternatives") {
		t.Errorf("rfc: %s", got)
	}
	if got := FormatValidationErrors(v.ValidateTaskPlan("We will add refunds.")); !strings.Contains(got, "no ordered sub-tasks") || !strings.Contains(got, "Repository") {
		t.Errorf("breakdown: %s", got)
	}
}

func TestArtifactManagerListsWhatIsOnDisk(t *testing.T) {
	base := t.TempDir()
	m := NewArtifactManager(base)
	dir := m.TaskArtifactDir("T-1")
	_ = os.MkdirAll(filepath.Join(dir, "stages"), 0o755)
	_ = os.MkdirAll(filepath.Join(dir, "evidence", ".tmp"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "stages", "04-techdoc_rfc.md"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "evidence", "A.mp4"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "evidence", ".tmp", "partial.mp4"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "PRD.md"), []byte("# PRD\nx"), 0o644)
	got := m.List("T-1")
	if len(got) != 3 || got[0].Path != "PRD.md" || got[0].Kind != "document" || got[1].Kind != "media" || got[2].StageID != "techdoc_rfc" {
		t.Fatalf("list: %+v", got)
	}
	if _, meta, err := m.ReadArtifact("T-1", ArtifactPRD); err != nil || meta.SHA256Hash == "" || meta.SizeBytes != 7 {
		t.Fatalf("metadata must come from disk: %+v %v", meta, err)
	}
}
