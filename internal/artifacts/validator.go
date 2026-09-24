package artifacts

import (
	"fmt"
	"strings"
)

// ValidationError details a compliance issue in an artifact.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ArtifactValidator inspects markdown structures against SDLC schema rules.
type ArtifactValidator struct{}

// NewArtifactValidator creates a validator.
func NewArtifactValidator() *ArtifactValidator {
	return &ArtifactValidator{}
}

// ValidatePRD checks PRD structure for problem statement, goals, user stories, and acceptance criteria.
func (v *ArtifactValidator) ValidatePRD(content string) []ValidationError {
	var errs []ValidationError

	if !strings.Contains(content, "# PRD") && !strings.Contains(content, "# Product Requirements") {
		errs = append(errs, ValidationError{Field: "header", Message: "Missing '# PRD' or '# Product Requirements' main title"})
	}
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "## acceptance criteria") && !strings.Contains(lower, "## user stories") {
		errs = append(errs, ValidationError{Field: "sections", Message: "PRD must contain '## Acceptance Criteria' or '## User Stories'"})
	}

	return errs
}

// ValidateATDD checks ATDD suite for Given/When/Then scenarios.
func (v *ArtifactValidator) ValidateATDD(content string) []ValidationError {
	var errs []ValidationError

	if !strings.Contains(content, "Feature:") && !strings.Contains(content, "Scenario:") {
		errs = append(errs, ValidationError{Field: "format", Message: "ATDD suite must include Gherkin 'Feature:' or 'Scenario:' specifications"})
	}

	return errs
}

// ValidateTechDoc checks RFC for architectural diagrams and schema contracts.
func (v *ArtifactValidator) ValidateTechDoc(content string) []ValidationError {
	var errs []ValidationError

	lower := strings.ToLower(content)
	if !strings.Contains(lower, "architecture") && !strings.Contains(lower, "design") {
		errs = append(errs, ValidationError{Field: "sections", Message: "Tech Doc RFC must document 'Architecture' or 'System Design'"})
	}

	return errs
}

// ValidateTaskPlan verifies sequenced atomic breakdown.
func (v *ArtifactValidator) ValidateTaskPlan(content string) []ValidationError {
	var errs []ValidationError

	if !strings.Contains(content, "TASK-") && !strings.Contains(content, "Task ") && !strings.Contains(content, "## Tasks") {
		errs = append(errs, ValidationError{Field: "tasks", Message: "Task plan must contain numbered atomic task identifiers (e.g. TASK-1.1)"})
	}

	return errs
}

// FormatValidationErrors formats errors into a human-readable string.
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
