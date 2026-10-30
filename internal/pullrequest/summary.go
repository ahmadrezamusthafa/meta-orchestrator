package pullrequest

import (
	"fmt"
	"regexp"
	"strings"
)

// The Summary is written for the reviewer from the change itself. The implementation report's own
// Summary describes the agent's last turn — a rebase, a CI fix, what is left for the operator — and
// covers every repository of the task, so a model summarizes each repository's diff against the
// task instead, and the report is only the fallback.

// SummaryInput is what one repository's summary is written from.
type SummaryInput struct {
	Title              string
	Description        string
	AcceptanceCriteria string
	Repo               string
	OtherRepos         []string
	Commits            []string
	ReportChanges      string // what the implementation report says changed in this repository
	Patch              string // the repository's diff against the target branch
	PatchTruncated     bool
}

// Prompt limits keep one summary call small; the diff is cut at a file boundary.
const (
	maxPromptPatch = 60_000
	maxPromptText  = 6_000
)

const summarySystem = `You write the "Summary" section of a pull request description for the engineer who reviews it.

Describe the change in this repository's diff: what it does and why, tied to the task's goal. Cover the behaviour a user or caller sees, the main components it touches, and any feature flag, toggle, migration, configuration or API contract it adds or changes. Mention notable tests only in one short clause.

Rules:
- Describe only what the diff contains. Say nothing the diff does not support; do not describe changes in other repositories of the task.
- Write for the reviewer: no mention of an agent, assistant, session, turn, push, pipeline run, rebase, conflict resolution, verification status, test evidence, or anything left for someone to do.
- Never reproduce secrets, credentials, tokens, connection strings or personal data, even when they appear in the input.
- Markdown, 2–6 short bullets or one short paragraph, under 150 words. No heading, no preamble, no code blocks.`

// SummaryPrompt returns the system and user prompts for one repository's summary.
func SummaryPrompt(in SummaryInput) (system, user string) {
	var b strings.Builder
	fmt.Fprintf(&b, "Task: %s\n", cleanTitle(in.Title))
	if d := clip(in.Description, maxPromptText); d != "" {
		fmt.Fprintf(&b, "\nTask description:\n%s\n", d)
	}
	if ac := clip(in.AcceptanceCriteria, maxPromptText); ac != "" {
		fmt.Fprintf(&b, "\nAcceptance criteria:\n%s\n", ac)
	}
	fmt.Fprintf(&b, "\nRepository of this pull request: %s\n", in.Repo)
	if len(in.OtherRepos) > 0 {
		fmt.Fprintf(&b, "Other repositories of the task (separate pull requests, not this one): %s\n", strings.Join(in.OtherRepos, ", "))
	}
	if len(in.Commits) > 0 {
		b.WriteString("\nCommits:\n")
		for _, c := range in.Commits {
			fmt.Fprintf(&b, "- %s\n", c)
		}
	}
	if rc := clip(in.ReportChanges, maxPromptText); rc != "" {
		fmt.Fprintf(&b, "\nThe implementer's notes on this repository's changes (may be stale; the diff wins):\n%s\n", rc)
	}
	patch, cut := in.Patch, in.PatchTruncated
	if len(patch) > maxPromptPatch {
		patch, cut = patch[:maxPromptPatch], true
		if i := strings.LastIndex(patch, "\ndiff --git "); i > 0 {
			patch = patch[:i+1]
		}
	}
	b.WriteString("\nDiff against the target branch")
	if cut {
		b.WriteString(" (cut short; the commits and notes describe the rest)")
	}
	fmt.Fprintf(&b, ":\n%s\n", patch)
	return summarySystem, b.String()
}

func clip(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		s = strings.TrimSpace(s[:n]) + " …"
	}
	return s
}

var leadingSummaryHeading = regexp.MustCompile(`(?i)^\s*(#{1,6}\s*|\*\*)summary(\*\*)?:?\s*\n`)

// CleanSummary normalizes the model's answer: no heading of its own and no code blocks. It
// returns "" when nothing usable is left.
func CleanSummary(raw string) string {
	s := strings.TrimSpace(raw)
	s = leadingSummaryHeading.ReplaceAllString(s, "")
	s = fencedBlock.ReplaceAllString(s+"\n", "")
	s = heading.ReplaceAllString(s, "**$2**")
	if strings.TrimSpace(s) == "" {
		return ""
	}
	return collapseBlank(s)
}
