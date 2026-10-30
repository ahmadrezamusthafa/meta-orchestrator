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
	Context            string // the problem the task solves, from the ticket or PRD
	Summary            string // what changed and why
	Details            string // what changed in this repository, per component or file
	Testing            string // how the change was verified
	AcceptanceCriteria string
	Risks              string
	Commits            []string
	Files              []FileChange
	Related            []Related // the task's pull requests in its other repositories
	Evidence           *Evidence // nil when no test evidence was recorded
}

// Related is a pull request of the same task in another repository; URL is empty until it is opened.
type Related struct {
	Repo string
	URL  string
}

// Evidence is the outcome of the task's recorded test evidence: results only, without the local
// videos, paths or environment (those stay in the orchestrator's Evidence tab).
type Evidence struct {
	Cases   []EvidenceCase
	Suites  []EvidenceSuite
	Covered int // acceptance cases of the spec that have evidence
	Total   int // acceptance cases of the spec; 0 when the task has none
}

// EvidenceCase is one recorded flow or acceptance case.
type EvidenceCase struct {
	ID, Kind, Title, Status string
}

// EvidenceSuite is one full test-suite run; Passed and Failed are nil when the run reported no counts.
type EvidenceSuite struct {
	Repository, Command, Status string
	Passed, Failed              *int
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

	b.WriteString("## Context\n")
	context := in.Context
	if strings.TrimSpace(context) == strings.TrimSpace(in.Summary) {
		context = "" // the summary already fell back to the task description
	}
	b.WriteString(or(context, "_See the linked ticket._") + "\n\n")

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
	if d := strings.TrimSpace(in.Details); d != "" {
		b.WriteString(d + "\n\n")
	}
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
	if len(in.Related) > 0 { // only on multi-repository tasks
		b.WriteString("## Related pull requests\n")
		for _, r := range in.Related {
			if r.URL != "" {
				fmt.Fprintf(&b, "- %s: %s\n", r.Repo, r.URL)
			} else {
				fmt.Fprintf(&b, "- %s: not opened yet\n", r.Repo)
			}
		}
		b.WriteString("\n")
	}

	b.WriteString("## Acceptance criteria\n")
	b.WriteString(or(in.AcceptanceCriteria, "_See the linked ticket._") + "\n\n")

	b.WriteString("## How to test\n")
	b.WriteString(or(in.Testing, "_List the steps a reviewer can follow to verify this change._") + "\n\n")

	if in.Evidence != nil {
		writeEvidence(&b, in.TaskID, in.Evidence)
	}

	b.WriteString("## Risk & rollback\n")
	b.WriteString(or(in.Risks, "_Low risk. Roll back by reverting this pull request._") + "\n\n")

	// The diff answers the test and documentation items; the rest is the reviewer's judgement.
	tests, docs := " ", " "
	for _, f := range in.Files {
		if f.Status == "deleted" {
			continue
		}
		if testFile.MatchString(f.Path) {
			tests = "x"
		}
		if docFile.MatchString(f.Path) {
			docs = "x"
		}
	}
	b.WriteString("## Checklist\n")
	fmt.Fprintf(&b, "- [%s] Unit / integration tests added or updated\n", tests)
	b.WriteString("- [ ] No secrets, credentials or customer PII in code, logs or fixtures\n")
	b.WriteString("- [ ] Feature flag / config changes documented\n")
	b.WriteString("- [ ] Backward compatible (API, DB schema, events)\n")
	fmt.Fprintf(&b, "- [%s] Documentation updated where needed\n\n", docs)

	fmt.Fprintf(&b, "---\n_`%s` → `%s` · generated by Meta-Orchestrator for %s_\n", in.SourceBranch, in.TargetBranch, in.TaskID)
	return b.String()
}

var (
	testFile = regexp.MustCompile(`(?i)(_test\.go$|_spec\.rb$|_test\.(py|rb|exs?)$|(^|/)test_[^/]+\.py$|\.(test|spec)\.[jt]sx?$|` +
		`(^|/)(test|tests|spec|__tests__|e2e|cypress)/|Tests?\.(java|kt|cs|php)$)`)
	docFile = regexp.MustCompile(`(?i)(\.(md|mdx|rst|adoc)$|(^|/)docs?/)`)
)

// writeEvidence renders the results of the recorded test evidence for the reviewer.
func writeEvidence(b *strings.Builder, taskID string, ev *Evidence) {
	b.WriteString("## Test evidence\n")
	if ev.Total > 0 {
		fmt.Fprintf(b, "%d of %d acceptance case(s) of the spec have recorded evidence.\n\n", ev.Covered, ev.Total)
	}
	if len(ev.Cases) > 0 {
		b.WriteString("| Case | Kind | Result |\n|---|---|---|\n")
		for _, c := range ev.Cases {
			name := c.ID
			if c.Title != "" {
				name += " — " + c.Title
			}
			fmt.Fprintf(b, "| %s | %s | %s |\n", tableCell(name), tableCell(c.Kind), tableCell(c.Status))
		}
		b.WriteString("\n")
	}
	if len(ev.Suites) > 0 {
		b.WriteString("**Test suites**\n\n| Repository | Command | Result |\n|---|---|---|\n")
		for _, s := range ev.Suites {
			result := s.Status
			if s.Passed != nil && s.Failed != nil {
				result += fmt.Sprintf(" (%d passed, %d failed)", *s.Passed, *s.Failed)
			}
			cmd := ""
			if s.Command != "" {
				cmd = "`" + strings.ReplaceAll(tableCell(s.Command), "`", "'") + "`"
			}
			fmt.Fprintf(b, "| %s | %s | %s |\n", tableCell(s.Repository), cmd, tableCell(result))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "_Videos of every case are on %s in Meta-Orchestrator (Evidence tab)._\n\n", taskID)
}

func tableCell(s string) string {
	return strings.ReplaceAll(strings.Join(strings.Fields(s), " "), "|", "\\|")
}

var heading = regexp.MustCompile(`(?m)^(#{1,6})\s+(.+?)\s*#*\s*$`)

// Section returns the body under the first markdown heading that contains any of the keywords
// (case-insensitive), up to the next heading of the same or higher level.
func Section(doc string, keywords ...string) string {
	return section(doc, func(title string) bool {
		for _, k := range keywords {
			if strings.Contains(title, strings.ToLower(k)) {
				return true
			}
		}
		return false
	})
}

func section(doc string, match func(lowerTitle string) bool) string {
	matches := heading.FindAllStringSubmatchIndex(doc, -1)
	for i, m := range matches {
		if !match(strings.ToLower(doc[m[4]:m[5]])) {
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

var (
	fencedBlock = regexp.MustCompile("(?ms)^[ \t]*(```|~~~)[^\n]*\n.*?^[ \t]*(```|~~~)[ \t]*$\n?")
	titleToken  = regexp.MustCompile(`[A-Za-z0-9_./-]+`)
)

// Changes returns what an implementation report says changed in repo: its changes section, narrowed
// to the sub-section naming repo when the report has one per repository. Code blocks are left out:
// the diff itself is in the pull request.
func Changes(doc, repo string) string {
	sec := Section(doc, "changes", "what changed", "implementation details")
	if sec == "" {
		return ""
	}
	if repo != "" {
		repo = strings.ToLower(repo)
		if sub := section(sec, func(title string) bool {
			for _, tok := range titleToken.FindAllString(title, -1) {
				if strings.Trim(tok, ".") == repo || strings.HasSuffix(tok, "/"+repo) {
					return true
				}
			}
			return false
		}); sub != "" {
			sec = sub
		}
	}
	sec = fencedBlock.ReplaceAllString(sec, "")
	if strings.TrimSpace(sec) == "" {
		return ""
	}
	return collapseBlank(sec)
}

// Test evidence is for the orchestrator's Evidence tab, not for the reviewer reading the pull
// request: the agent's report mentions the recorder, its videos and storyboard, local evidence
// paths and how its environment was set up. WithoutEvidence drops those parts of a stage document
// before its sections become the description.

// evidenceHeading matches a section about the evidence ("## Test evidence", "### Evidence videos").
var evidenceHeading = regexp.MustCompile(`(?i)\bevidence\b`)

// evidenceLine matches a line that talks about recording evidence rather than about the change.
var evidenceLine = regexp.MustCompile(`(?i)(test[ -]evidence|evidence (video|recorder|run|turn|dir|directory|folder|tab|file|case)s?\b|` +
	`record(ed|ing|s)? (the |no |any |all )?(test )?(evidence|videos?)\b|evidence\.json|record-evidence|storyboard|` +
	`\.(mp4|webm)\b|summary\.png|\.sdlc/|test environment|throwaway (database|container))`)

// WithoutEvidence returns doc without its evidence sections and without the lines (and the
// sub-items under them) that report on recording evidence.
func WithoutEvidence(doc string) string {
	lines := strings.Split(doc, "\n")
	out := make([]string, 0, len(lines))
	skipLevel := 0   // inside an evidence section of this heading level
	dropIndent := -1 // dropping the sub-items of a dropped list item indented this much
	for _, line := range lines {
		if m := heading.FindStringSubmatch(line); m != nil {
			level := len(m[1])
			if skipLevel > 0 && level > skipLevel {
				continue
			}
			skipLevel, dropIndent = 0, -1
			if evidenceHeading.MatchString(m[2]) {
				skipLevel = level
				continue
			}
			out = append(out, line)
			continue
		}
		if skipLevel > 0 {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if dropIndent >= 0 {
			if trimmed != "" && indent > dropIndent {
				continue
			}
			dropIndent = -1
		}
		if trimmed != "" && evidenceLine.MatchString(line) {
			if listItem.MatchString(trimmed) {
				dropIndent = indent
			}
			continue
		}
		out = append(out, line)
	}
	return collapseBlank(strings.Join(out, "\n"))
}

var listItem = regexp.MustCompile(`^([-*+]|\d+[.)])\s`)

var blankRun = regexp.MustCompile(`\n{3,}`)

func collapseBlank(s string) string {
	return strings.TrimSpace(blankRun.ReplaceAllString(s, "\n\n")) + "\n"
}
