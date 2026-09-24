package techdoc

import (
	"fmt"
	"strings"
	"time"
)

// RFCContractSpecification holds data to generate TECH_DOC_RFC.md.
type RFCContractSpecification struct {
	FeatureTitle    string   `json:"feature_title"`
	TargetRepos     []string `json:"target_repos"`
	DatabaseChanges []string `json:"database_changes"`
	APIEndpoints    []string `json:"api_endpoints"`
	ImpactedClasses []string `json:"impacted_classes"`
}

// RFCGenerator creates formal technical architecture RFCs.
type RFCGenerator struct{}

// NewRFCGenerator creates an RFC generator.
func NewRFCGenerator() *RFCGenerator {
	return &RFCGenerator{}
}

// GenerateRFC produces markdown compliant with PRD Section 6.
func (g *RFCGenerator) GenerateRFC(spec *RFCContractSpecification) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Technical Design Document & RFC: %s\n\n", spec.FeatureTitle))
	b.WriteString(fmt.Sprintf("> **Author:** System Architect Agent  \n> **Date:** %s  \n> **Review Gate:** `GATE_TECH_DOC_REVIEW`\n\n", time.Now().Format("2006-01-02")))

	b.WriteString("## 1. System Architecture & Impact Radius\n")
	b.WriteString("The following repositories and components will be affected by this change:\n")
	for _, repo := range spec.TargetRepos {
		b.WriteString(fmt.Sprintf("- `%s`\n", repo))
	}
	b.WriteString("\n")

	b.WriteString("### Impacted AST Classes & Interfaces\n")
	for _, cls := range spec.ImpactedClasses {
		b.WriteString(fmt.Sprintf("- `%s`\n", cls))
	}
	b.WriteString("\n")

	b.WriteString("## 2. API Contract Specifications\n")
	for _, ep := range spec.APIEndpoints {
		b.WriteString(fmt.Sprintf("```http\n%s\n```\n\n", ep))
	}

	b.WriteString("## 3. Database Schema & Migration Strategy\n")
	for _, change := range spec.DatabaseChanges {
		b.WriteString(fmt.Sprintf("- %s\n", change))
	}
	b.WriteString("\n")

	b.WriteString("## 4. Sequence Diagram\n")
	b.WriteString("```mermaid\nsequenceDiagram\n")
	b.WriteString("  actor Client as User / Frontend\n")
	b.WriteString("  participant API as Backend Gateway\n")
	b.WriteString("  participant DB as Database / Cache\n")
	b.WriteString("  Client->>API: POST /api/v1/resource\n")
	b.WriteString("  API->>DB: Query / Mutate Entity\n")
	b.WriteString("  DB-->>API: Result\n")
	b.WriteString("  API-->>Client: 200 OK + Payload\n")
	b.WriteString("```\n")

	return b.String()
}
