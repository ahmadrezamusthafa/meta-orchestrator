package uat

import (
	"fmt"
	"strings"
	"time"
)

// UATSpecification provides structured data to generate UAT_PREPARATION.md.
type UATSpecification struct {
	FeatureTitle     string              `json:"feature_title"`
	BusinessGoal     string              `json:"business_goal"`
	TargetEnvironment string             `json:"target_environment"`
	TestCredentials  map[string]string   `json:"test_credentials"`
	UserClickPaths   []string            `json:"user_click_paths"`
	EdgeCasesChecked []string            `json:"edge_cases_checked"`
	VerificationRubrics []string         `json:"verification_rubrics"`
}

// UATGenerator formats human-readable User Acceptance Testing manuals.
type UATGenerator struct{}

// NewUATGenerator creates a generator.
func NewUATGenerator() *UATGenerator {
	return &UATGenerator{}
}

// GenerateUATDocument renders markdown compliant with PRD Section 6.1.
func (g *UATGenerator) GenerateUATDocument(spec *UATSpecification) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# User Acceptance Testing (UAT) Guide: %s\n\n", spec.FeatureTitle))
	b.WriteString(fmt.Sprintf("> **Generated at:** %s  \n", time.Now().Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("> **Environment:** `%s`\n\n", spec.TargetEnvironment))

	b.WriteString("## 1. Feature Summary & Objectives\n")
	b.WriteString(fmt.Sprintf("%s\n\n", spec.BusinessGoal))

	b.WriteString("## 2. Test Credentials & Seeded Accounts\n")
	if len(spec.TestCredentials) > 0 {
		b.WriteString("| Role | Username / Email | Password |\n|---|---|---|\n")
		for role, cred := range spec.TestCredentials {
			b.WriteString(fmt.Sprintf("| %s | `%s` | `password123` |\n", role, cred))
		}
	} else {
		b.WriteString("- Default demo account active.\n")
	}
	b.WriteString("\n")

	b.WriteString("## 3. Step-by-Step User Click Paths\n")
	for i, step := range spec.UserClickPaths {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, step))
	}
	b.WriteString("\n")

	b.WriteString("## 4. Edge Cases Verified\n")
	for _, edge := range spec.EdgeCasesChecked {
		b.WriteString(fmt.Sprintf("- [x] %s\n", edge))
	}
	b.WriteString("\n")

	b.WriteString("## 5. Pass / Fail Verification Rubrics\n")
	for _, rubric := range spec.VerificationRubrics {
		b.WriteString(fmt.Sprintf("- [ ] %s\n", rubric))
	}
	b.WriteString("\n---\n*Authored autonomously by Meta-Orchestrator QA Agent*\n")

	return b.String()
}
