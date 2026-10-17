package router

import "strings"

// taskTypeRules are checked in order; the first category with a matching keyword wins.
var taskTypeRules = []struct {
	category string
	keywords []string
}{
	{"architecture", []string{"architecture", "architectural", "system design", "design new", "epic", "event-sourcing", "rearchitect"}},
	{"migration", []string{"migration", "migrate", "schema change", "upgrade to"}},
	{"docs", []string{"readme", "typo", "documentation", "docs", "changelog", "comment"}},
	{"bugfix", []string{"bug", "fix", "crash", "regression", "hotfix", "error", "null pointer"}},
	{"crud", []string{"crud", "endpoint", "rest api", "create ", "update ", "delete ", "list ", "form"}},
	{"refactor", []string{"refactor", "cleanup", "clean up", "rename"}},
	{"test", []string{"test", "coverage", "atdd", "e2e"}},
	{"ui", []string{"ui", "css", "layout", "component", "style"}},
}

// ClassifyTaskType maps a request to a recurring task category used by the feedback loop.
func ClassifyTaskType(title string, description string) string {
	combined := " " + strings.ToLower(title+" "+description) + " "
	for _, rule := range taskTypeRules {
		for _, kw := range rule.keywords {
			if strings.Contains(combined, kw) {
				return rule.category
			}
		}
	}
	return "general"
}
