package atdd

import (
	"strings"
	"testing"
)

func TestATDDGeneratorPlaywrightSpec(t *testing.T) {
	cases := []Case{{
		ID: "ABC-TC-001", Title: "Admin's \"refund\" button\nworks `now` ${x} \\ </script>",
		Preconditions:   []string{"I am signed in as 'Super Admin'."},
		Steps:           []string{"1. Open /billing?x='1'", "2. Click \"Refund\""},
		ExpectedResults: []string{"1. Toast says 'Refunded'"},
	}, {ID: "ABC-TC-002", Title: "No steps yet"}}

	spec := NewATDDGenerator().GeneratePlaywrightSpec("Billing 'Refunds'", cases)

	for _, want := range []string{
		"import { test } from '@playwright/test';",
		`test.describe("Billing 'Refunds' acceptance suite (ATDD)", () => {`,
		`test("ABC-TC-001: Admin's \"refund\" button\nworks ` + "`now`" + ` ${x} \\ \u003c/script\u003e", async () => {`,
		`await test.step("1. Open /billing?x='1'", async () => notImplemented("1. Open /billing?x='1'"));`,
		`await test.step("2. Click \"Refund\"", async () => notImplemented("2. Click \"Refund\""));`,
		`await test.step("Expect: 1. Toast says 'Refunded'", async () => notImplemented("assertion: 1. Toast says 'Refunded'"));`,
		`description: "I am signed in as 'Super Admin'."`,
		`notImplemented("case ABC-TC-002 has no steps");`,
		"throw new Error('Not implemented: ' + what);",
	} {
		if !strings.Contains(spec, want) {
			t.Errorf("spec missing %s\n---\n%s", want, spec)
		}
	}
	// A raw newline inside a literal would break the file; every case line must stay on one line.
	for _, line := range strings.Split(spec, "\n") {
		if strings.HasPrefix(line, "works") {
			t.Fatalf("unescaped newline in output:\n%s", spec)
		}
	}
	if !IsWebCase(Case{Platform: "WEB"}) || !IsWebCase(Case{}) || IsWebCase(Case{Platform: "API"}) || IsWebCase(Case{Platform: "android"}) {
		t.Fatal("IsWebCase misclassifies platforms")
	}
}
