package router

import (
	"strings"
	"testing"
)

func TestClassifyTaskType(t *testing.T) {
	for in, want := range map[string]string{
		"Fix typo in README":                  "docs",
		"Hotfix: null pointer in checkout":    "bugfix",
		"Migrate invoices to the new schema":  "migration",
		"Refactor the payment client":         "refactor",
		"Something with no recognisable kind": "general",
	} {
		if got := ClassifyTaskType(in, ""); got != want {
			t.Errorf("ClassifyTaskType(%q) = %s, want %s", in, got, want)
		}
	}
}

var platformRepos = []*DiscoveredRepo{
	{Name: "billing", Role: "backend", Stack: []string{"Go", "PostgreSQL"}},
	{Name: "billing-internal-fe", Role: "frontend", Stack: []string{"Vue", "TypeScript"}},
	{Name: "api-contracts", Role: "contracts", Stack: []string{"OpenAPI"}},
	{Name: "infra-live", Role: "infrastructure", Stack: []string{"Terragrunt"}},
	{Name: "qa-suite", Role: "automation-test", Stack: []string{"RSpec"}},
}

func names(ms []RepoMatch) string {
	var out []string
	for _, m := range ms {
		out = append(out, m.Name)
	}
	return strings.Join(out, ",")
}

func TestFindImpactedReposRanksByEvidence(t *testing.T) {
	cases := []struct{ text, want string }{
		// A repository named in the task comes first; role words add the frontend.
		{"Add refund endpoint to billing and a refund button on the invoice page", "billing,billing-internal-fe"},
		{"Update the OpenAPI schema for invoices", "api-contracts"},
		{"Deploy the new queue with terragrunt", "infra-live,billing"},
		{"Write playwright e2e coverage for checkout", "qa-suite"},
		// Substrings are not words: "build", "rapid" and "guide" mention neither "ui" nor "api".
		{"Rapid build guide", ""},
	}
	for _, c := range cases {
		if got := names(FindImpactedRepos(platformRepos, c.text)); got != c.want {
			t.Errorf("%q → %q, want %q", c.text, got, c.want)
		}
	}
	m := FindImpactedRepos(platformRepos, "billing api")[0]
	if m.Name != "billing" || m.Reasons[0] != "named in the task" {
		t.Errorf("top match = %+v", m)
	}
}
