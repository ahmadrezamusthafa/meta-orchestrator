package router

import (
	"sort"
	"strings"
)

// DiscoveredRepo is a registered repository as repo suggestions see it.
type DiscoveredRepo struct {
	Name  string   `json:"name"`
	Role  string   `json:"role"`  // project role: frontend, backend, contracts, infrastructure, automation-test, …
	Stack []string `json:"stack"` // detected technologies, e.g. "Go", "Vue"
}

// RepoMatch is a repository a task likely touches, and why.
type RepoMatch struct {
	Name    string   `json:"name"`
	Score   int      `json:"score"`
	Reasons []string `json:"reasons"`
}

// roleKeywords are whole words in a task that point at a repository role.
var roleKeywords = map[string][]string{
	"frontend":        {"ui", "ux", "page", "screen", "frontend", "button", "form", "css", "component", "modal", "dashboard", "layout"},
	"backend":         {"api", "endpoint", "backend", "database", "db", "migration", "webhook", "queue", "worker", "server", "sql"},
	"contracts":       {"contract", "contracts", "schema", "openapi", "swagger", "protobuf", "proto", "grpc"},
	"infrastructure":  {"infra", "infrastructure", "deploy", "deployment", "terraform", "terragrunt", "opentofu", "helm", "kubernetes", "k8s"},
	"automation-test": {"e2e", "rspec", "playwright", "cypress", "regression", "automation"},
}

// genericNameParts say nothing about what a repository holds ("billing-service" is about billing).
var genericNameParts = map[string]bool{"service": true, "services": true, "api": true, "app": true, "core": true, "web": true,
	"internal": true, "fe": true, "be": true, "lib": true, "repo": true, "main": true, "server": true, "client": true}

func words(s string) []string {
	return strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// FindImpactedRepos scores each repository against the task text: its full name (strongest), the
// meaningful words of its name, its stack, and words that point at its role. Repositories with no
// evidence are left out — there is no "all of them" fallback.
func FindImpactedRepos(repos []*DiscoveredRepo, text string) []RepoMatch {
	lower := strings.ToLower(text)
	have := map[string]bool{}
	for _, w := range words(text) {
		have[w] = true
	}
	var out []RepoMatch
	for _, r := range repos {
		m := RepoMatch{Name: r.Name}
		if name := strings.ToLower(r.Name); len(name) > 2 && containsWord(lower, name) {
			m.Score += 10
			m.Reasons = append(m.Reasons, "named in the task")
		} else {
			for _, part := range words(r.Name) {
				if len(part) > 2 && !genericNameParts[part] && have[part] {
					m.Score += 4
					m.Reasons = append(m.Reasons, "name mentions "+part)
				}
			}
		}
		for _, tech := range r.Stack {
			if t := strings.ToLower(tech); len(t) > 2 && have[t] {
				m.Score += 3
				m.Reasons = append(m.Reasons, "uses "+tech)
			}
		}
		hits := 0
		for _, kw := range roleKeywords[r.Role] {
			if have[kw] && hits < 3 {
				hits++
				m.Score += 2
				m.Reasons = append(m.Reasons, r.Role+" work ("+kw+")")
			}
		}
		if m.Score > 0 {
			out = append(out, m)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}

// containsWord reports whether s contains phrase delimited by non-alphanumeric characters.
func containsWord(s, phrase string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], phrase)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(phrase)
		if (start == 0 || !isAlnum(s[start-1])) && (end == len(s) || !isAlnum(s[end])) {
			return true
		}
		i = start + 1
	}
}

func isAlnum(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }
