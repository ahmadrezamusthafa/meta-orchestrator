package atdd

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/ast"
)

// ATDDTestCase defines an individual test scenario mapped to PRD criteria.
type ATDDTestCase struct {
	ScenarioName  string   `json:"scenario_name"`
	TargetRoute   string   `json:"target_route"`
	ActionSteps   []string `json:"action_steps"`
	AssertElement string   `json:"assert_element"`
	ExpectedText  string   `json:"expected_text"`
}

// ATDDGenerator builds executable Playwright end-to-end test suites.
type ATDDGenerator struct{}

// NewATDDGenerator creates a generator.
func NewATDDGenerator() *ATDDGenerator {
	return &ATDDGenerator{}
}

// GeneratePlaywrightSpec constructs Playwright TypeScript test file.
func (g *ATDDGenerator) GeneratePlaywrightSpec(featureName string, cases []ATDDTestCase, astSlice *ast.ASTContextSlice) string {
	var b strings.Builder

	b.WriteString("import { test, expect } from '@playwright/test';\n\n")
	b.WriteString(fmt.Sprintf("test.describe('%s Acceptance Suite (ATDD)', () => {\n", featureName))

	for _, c := range cases {
		b.WriteString(fmt.Sprintf("  test('%s', async ({ page }) => {\n", c.ScenarioName))
		if c.TargetRoute != "" {
			b.WriteString(fmt.Sprintf("    await page.goto('%s');\n", c.TargetRoute))
		}
		for _, step := range c.ActionSteps {
			b.WriteString(fmt.Sprintf("    // %s\n", step))
		}
		if c.AssertElement != "" {
			b.WriteString(fmt.Sprintf("    const el = page.locator('%s');\n", c.AssertElement))
			b.WriteString("    await expect(el).toBeVisible();\n")
			if c.ExpectedText != "" {
				b.WriteString(fmt.Sprintf("    await expect(el).toContainText('%s');\n", c.ExpectedText))
			}
		}
		b.WriteString("  });\n\n")
	}

	b.WriteString("});\n")
	return b.String()
}
