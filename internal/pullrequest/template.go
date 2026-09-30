// Package pullrequest builds pull requests in one standard format and opens them on the hosting
// provider (Bitbucket Cloud or GitHub).
package pullrequest

import (
	"fmt"
	"regexp"
	"strings"
)

// FileChange is one changed path in the pull request.
type FileChange struct {
	Path      string `json:"path"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
}

// Input is everything the standard description is built from.
type Input struct {
	TaskID             string
	JiraKey            string
	JiraURL            string
	Title              string
	TaskType           string // router task category (bugfix, crud, refactor, …)
	Repo               string
	SourceBranch       string
	TargetBranch       string
	Summary            string // what changed and why
	Testing            string // how the change was verified
	AcceptanceCriteria string
	Risks              string
	Commits            []string
	Files              []FileChange
}

// conventionalType maps the router's task category to a Conventional Commits type.
func conventionalType(taskType string) string {
	switch taskType {
	case "bugfix":
		return "fix"
	case "refactor", "migration":
		return "refactor"
	case "docs":
		return "docs"
	case "test":
		return "test"
	}
	return "feat"
}

var leadingKey = regexp.MustCompile(`^\s*\[[A-Za-z][A-Za-z0-9]*-\d+\]\s*`)

// cleanTitle drops a leading "[KEY-1]" so the key is not repeated.
func cleanTitle(title string) string {
	return strings.TrimSpace(leadingKey.ReplaceAllString(title, ""))
}

// Title is the standard pull request title: "[KEY-1] type: summary" (the task ID stands in for
// a missing JIRA key).
func Title(in Input) string {
	key := in.JiraKey
	if key == "" {
		key = in.TaskID
	}
	return fmt.Sprintf("[%s] %s: %s", key, conventionalType(in.TaskType), cleanTitle(in.Title))
}

// CommitMessage is the standard message for committing pending work before the push.
func CommitMessage(in Input) string {
	scope := in.JiraKey
	if scope == "" {
		scope = in.TaskID
	}
	return fmt.Sprintf("%s(%s): %s", conventionalType(in.TaskType), scope, cleanTitle(in.Title))
}

// maxFileRows keeps the change table readable on large pull requests.
const maxFileRows = 40

// Body renders the standard pull request description. Every section is always present so
// reviewers find the same structure on every pull request; empty ones say so explicitly.
func Body(in Input) string {
	var b strings.Builder
	or := func(s, fallback string) string {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
		return fallback
	}

	b.WriteString("## Ticket\n")
	switch {
	case in.JiraKey != "" && in.JiraURL != "":
		fmt.Fprintf(&b, "[%s](%s)\n\n", in.JiraKey, in.JiraURL)
	case in.JiraKey != "":
		fmt.Fprintf(&b, "%s\n\n", in.JiraKey)
	default:
		fmt.Fprintf(&b, "%s (no JIRA ticket linked)\n\n", in.TaskID)
	}

	b.WriteString("## Summary\n")
	b.WriteString(or(in.Summary, "_Describe what changed and why._") + "\n\n")

	b.WriteString("## Type of change\n")
	typ := conventionalType(in.TaskType)
	for _, opt := range []struct{ key, label string }{
		{"feat", "New feature"}, {"fix", "Bug fix"}, {"refactor", "Refactor / migration"},
		{"docs", "Documentation"}, {"test", "Tests only"},
	} {
		mark := " "
		if opt.key == typ {
			mark = "x"
		}
		fmt.Fprintf(&b, "- [%s] %s\n", mark, opt.label)
	}
	b.WriteString("\n")

	b.WriteString("## Changes\n")
	if len(in.Files) == 0 {
		b.WriteString("_No file changes detected._\n\n")
	} else {
		adds, dels := 0, 0
		for _, f := range in.Files {
			adds += f.Additions
			dels += f.Deletions
		}
		fmt.Fprintf(&b, "%d file(s), +%d / −%d\n\n", len(in.Files), adds, dels)
		b.WriteString("| Status | File | + | − |\n|---|---|---:|---:|\n")
		for i, f := range in.Files {
			if i == maxFileRows {
				fmt.Fprintf(&b, "| … | %d more file(s) | | |\n", len(in.Files)-maxFileRows)
				break
			}
			fmt.Fprintf(&b, "| %s | `%s` | %d | %d |\n", f.Status, f.Path, f.Additions, f.Deletions)
		}
		b.WriteString("\n")
	}
	if len(in.Commits) > 0 { // plain markdown: Bitbucket strips HTML such as <details>
		b.WriteString("**Commits**\n\n")
		for _, c := range in.Commits {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Acceptance criteria\n")
	b.WriteString(or(in.AcceptanceCriteria, "_See the linked ticket._") + "\n\n")

	b.WriteString("## How to test\n")
	b.WriteString(or(in.Testing, "_List the steps a reviewer can follow to verify this change._") + "\n\n")

	b.WriteString("## Risk & rollback\n")
	b.WriteString(or(in.Risks, "_Low risk. Roll back by reverting this pull request._") + "\n\n")

	b.WriteString("## Checklist\n")
	b.WriteString("- [ ] Unit / integration tests added or updated\n")
	b.WriteString("- [ ] No secrets, credentials or customer PII in code, logs or fixtures\n")
	b.WriteString("- [ ] Feature flag / config changes documented\n")
	b.WriteString("- [ ] Backward compatible (API, DB schema, events)\n")
	b.WriteString("- [ ] Documentation updated where needed\n\n")

	fmt.Fprintf(&b, "---\n_`%s` → `%s` · generated by Meta-Orchestrator for %s_\n", in.SourceBranch, in.TargetBranch, in.TaskID)
	return b.String()
}

var heading = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*#*\s*$`)

// Section returns the body under the first markdown heading that contains any of the keywords
// (case-insensitive), up to the next heading of the same or higher level.
func Section(doc string, keywords ...string) string {
	matches := heading.FindAllStringSubmatchIndex(doc, -1)
	for i, m := range matches {
		title := strings.ToLower(doc[m[4]:m[5]])
		hit := false
		for _, k := range keywords {
			hit = hit || strings.Contains(title, strings.ToLower(k))
		}
		if !hit {
			continue
		}
		level := m[3] - m[2]
		end := len(doc)
		for _, n := range matches[i+1:] {
			if n[3]-n[2] <= level {
				end = n[0]
				break
			}
		}
		if s := strings.TrimSpace(doc[m[1]:end]); s != "" {
			return s
		}
	}
	return ""
}
