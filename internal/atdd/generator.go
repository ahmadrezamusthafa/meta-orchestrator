package atdd

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ATDDGenerator builds Playwright acceptance-test scaffolds from ATDD cases.
type ATDDGenerator struct{}

// NewATDDGenerator creates a generator.
func NewATDDGenerator() *ATDDGenerator {
	return &ATDDGenerator{}
}

// jsString renders s as a JavaScript string literal. JSON strings are valid JS literals, and
// encoding/json escapes quotes, backslashes, newlines, U+2028/U+2029 and </script>-like markup.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// IsWebCase reports whether a case is driven through a browser (not API or mobile).
func IsWebCase(c Case) bool {
	return !c.IsAPI() && !c.IsMobile()
}

// GeneratePlaywrightSpec writes a Playwright scaffold with one test per case. Every step and
// expected result becomes a test.step that throws "not implemented", so the scaffold fails until
// each step is written — it can never pass by accident.
func (g *ATDDGenerator) GeneratePlaywrightSpec(featureName string, cases []Case) string {
	var b strings.Builder
	b.WriteString("import { test } from '@playwright/test';\n\n")
	b.WriteString("// Scaffold generated from the ATDD cases. Replace each notImplemented() with the real\n")
	b.WriteString("// interaction or assertion; until then every test fails.\n")
	b.WriteString("function notImplemented(what: string): never {\n  throw new Error('Not implemented: ' + what);\n}\n\n")
	fmt.Fprintf(&b, "test.describe(%s, () => {\n", jsString(featureName+" acceptance suite (ATDD)"))
	for _, c := range cases {
		name := c.ID
		if c.Title != "" {
			name += ": " + c.Title
		}
		fmt.Fprintf(&b, "  test(%s, async () => {\n", jsString(name))
		for _, p := range c.Preconditions {
			fmt.Fprintf(&b, "    test.info().annotations.push({ type: 'precondition', description: %s });\n", jsString(p))
		}
		steps := 0
		for _, s := range c.Steps {
			fmt.Fprintf(&b, "    await test.step(%s, async () => notImplemented(%s));\n", jsString(s), jsString(s))
			steps++
		}
		for _, e := range c.ExpectedResults {
			fmt.Fprintf(&b, "    await test.step(%s, async () => notImplemented(%s));\n", jsString("Expect: "+e), jsString("assertion: "+e))
			steps++
		}
		if steps == 0 {
			fmt.Fprintf(&b, "    notImplemented(%s);\n", jsString("case "+c.ID+" has no steps"))
		}
		b.WriteString("  });\n\n")
	}
	b.WriteString("});\n")
	return b.String()
}
