package artifacts

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/techdoc"
)

// ValidationError details a compliance issue in an artifact.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ArtifactValidator checks stage documents against what their stage brief asks for. The checks
// are advisory: they name what looks missing, they never block a stage.
type ArtifactValidator struct{}

// NewArtifactValidator creates a validator.
func NewArtifactValidator() *ArtifactValidator {
	return &ArtifactValidator{}
}

// prdSections are what the prd_discovery brief asks for: "goals, scope and non-goals, acceptance
// criteria, assumptions and open questions".
var prdSections = []techdoc.Section{
	{Name: "Goals", Keywords: []string{"goal", "objective"}},
	{Name: "Scope", Keywords: []string{"scope"}},
	{Name: "Non-goals", Keywords: []string{"non-goal", "non goal", "out of scope", "out-of-scope"}},
	{Name: "Acceptance criteria", Keywords: []string{"acceptance criteria", "acceptance criterion"}},
	{Name: "Assumptions", Keywords: []string{"assumption"}},
	{Name: "Open questions", Keywords: []string{"open question", "questions"}},
}

func missingAsErrors(field string, missing []string, what string) []ValidationError {
	var errs []ValidationError
	for _, m := range missing {
		errs = append(errs, ValidationError{Field: field, Message: fmt.Sprintf("no %s section (%s)", m, what)})
	}
	return errs
}

// ValidatePRD checks the requirement clarification covers what the stage asks for.
func (v *ArtifactValidator) ValidatePRD(content string) []ValidationError {
	return missingAsErrors("sections", techdoc.MissingSections(content, prdSections), "no heading or labelled line mentions it")
}

var (
	atddPriorities = map[string]bool{"P0": true, "P1": true, "P2": true}
	atddLevels     = map[string]bool{"SIT": true, "SUT": true}
	atddPlatforms  = map[string]bool{"WEB": true, "MOBILE": true, "API": true}
)

// ValidateATDD checks the ```atdd-cases block the ATDD stage must emit: it parses, and every case
// has a title, steps and expected results, and a priority, test level and platform from the brief.
func (v *ArtifactValidator) ValidateATDD(content string) []ValidationError {
	cases, err := atdd.ParseBlock(content)
	if err != nil {
		return []ValidationError{{Field: atdd.CasesFence, Message: err.Error()}}
	}
	if len(cases) == 0 {
		return []ValidationError{{Field: atdd.CasesFence, Message: "the block lists no cases with an id"}}
	}
	var errs []ValidationError
	seen := map[string]bool{}
	for _, c := range cases {
		var gaps []string
		if seen[c.ID] {
			gaps = append(gaps, "duplicate id")
		}
		seen[c.ID] = true
		if c.Title == "" {
			gaps = append(gaps, "no title")
		}
		if len(c.Steps) == 0 {
			gaps = append(gaps, "no steps")
		}
		if len(c.ExpectedResults) == 0 {
			gaps = append(gaps, "no expected results")
		}
		if !atddPriorities[strings.ToUpper(c.Priority)] {
			gaps = append(gaps, fmt.Sprintf("priority %q is not P0/P1/P2", c.Priority))
		}
		if !atddLevels[strings.ToUpper(c.TestLevel)] {
			gaps = append(gaps, fmt.Sprintf("test_level %q is not SIT/SUT", c.TestLevel))
		}
		if !atddPlatforms[strings.ToUpper(c.Platform)] {
			gaps = append(gaps, fmt.Sprintf("platform %q is not WEB/MOBILE/API", c.Platform))
		}
		if len(gaps) > 0 {
			errs = append(errs, ValidationError{Field: c.ID, Message: strings.Join(gaps, ", ")})
		}
	}
	return errs
}

// ValidateTechDoc checks the technical design covers approach, components, data/API changes,
// risks and alternatives.
func (v *ArtifactValidator) ValidateTechDoc(content string) []ValidationError {
	return missingAsErrors("sections", techdoc.MissingSections(content, techdoc.RFCSections), "no heading or labelled line mentions it")
}

// ValidateTaskPlan checks the breakdown is a list of ordered sub-tasks naming repositories, files
// and how each is verified.
func (v *ArtifactValidator) ValidateTaskPlan(content string) []ValidationError {
	var errs []ValidationError
	if techdoc.CountSteps(content) == 0 {
		errs = append(errs, ValidationError{Field: "tasks", Message: "no ordered sub-tasks (a numbered list, checklist, numbered table or headed steps)"})
	}
	return append(errs, missingAsErrors("tasks", techdoc.MissingMentions(content, techdoc.BreakdownSections), "the sub-tasks never mention it")...)
}

// ValidateStage runs the check for a stage's document; stages without a check return nil.
func (v *ArtifactValidator) ValidateStage(stage, content string) (errs []ValidationError, checked bool) {
	switch stage {
	case "prd_discovery":
		return v.ValidatePRD(content), true
	case "atdd_creation":
		return v.ValidateATDD(content), true
	case "techdoc_rfc":
		return v.ValidateTechDoc(content), true
	case "task_breakdown":
		return v.ValidateTaskPlan(content), true
	}
	return nil, false
}

// FormatValidationErrors formats errors into a human-readable list.
func FormatValidationErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return ""
	}
	var b strings.Builder
	for _, e := range errs {
		b.WriteString(fmt.Sprintf("- [%s] %s\n", e.Field, e.Message))
	}
	return b.String()
}
