// Package techdoc frames the technical-design (techdoc_rfc) and task-breakdown stage documents:
// the sections their stage briefs ask for, and a title and metadata header placed around the
// agent's own content. It never writes design content of its own.
package techdoc

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Section is a part a stage document is asked to cover, recognised by any of its keywords in a
// heading or a labelled line ("**Risks**", "Risks:").
type Section struct {
	Name     string
	Keywords []string
}

// RFCSections are what the techdoc_rfc brief asks for: "approach, components, data/API changes,
// risks and alternatives considered".
var RFCSections = []Section{
	{"Approach", []string{"approach", "solution", "overview", "proposal", "design"}},
	{"Components", []string{"component", "module", "service", "architecture"}},
	{"Data / API changes", []string{"data", "api", "schema", "migration", "endpoint", "contract"}},
	{"Risks", []string{"risk"}},
	{"Alternatives considered", []string{"alternative", "options considered", "trade-off", "tradeoff"}},
}

// BreakdownSections are what each task_breakdown sub-task is asked to carry: "the repository,
// files and how it will be verified".
var BreakdownSections = []Section{
	{"Repository", []string{"repo"}},
	{"Files", []string{"file", ".go", ".ts", ".vue", ".rb", ".py", ".php", ".js", ".java", ".kt", ".sql"}},
	{"Verification", []string{"verif", "test", "check"}},
}

var labelLine = regexp.MustCompile(`^\s*(#{1,6}\s+.+|\*\*[^*]+\*\*.*|__[^_]+__.*|[^|]{1,60}:\s*.*|\|.+\|)\s*$`)

// Labels returns the lowercased headings, labelled lines and table rows of a markdown document.
func Labels(content string) []string {
	var out []string
	for _, line := range strings.Split(content, "\n") {
		if labelLine.MatchString(line) {
			out = append(out, strings.ToLower(line))
		}
	}
	return out
}

// MissingSections lists the sections no heading, labelled line or table row mentions.
func MissingSections(content string, sections []Section) []string {
	labels := Labels(content)
	var missing []string
	for _, s := range sections {
		found := false
		for _, l := range labels {
			for _, k := range s.Keywords {
				if strings.Contains(l, k) {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			missing = append(missing, s.Name)
		}
	}
	return missing
}

// MissingMentions lists the sections the document mentions nowhere.
func MissingMentions(content string, sections []Section) []string {
	lower := strings.ToLower(content)
	var missing []string
	for _, s := range sections {
		found := false
		for _, k := range s.Keywords {
			if strings.Contains(lower, k) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, s.Name)
		}
	}
	return missing
}

// orderedStep matches a numbered list item, a checklist item, or a numbered table row.
var orderedStep = regexp.MustCompile(`(?mi)^\s*(\d+[.)]\s+\S|[-*]\s+\[[ xX]\]\s+\S|\|\s*\d+\s*\||#{2,4}\s+(\d+|task|step|sub-?task)\b)`)

// CountSteps counts the ordered sub-tasks of a breakdown.
func CountSteps(content string) int {
	return len(orderedStep.FindAllString(content, -1))
}

// Meta is what the header says about a stage document.
type Meta struct {
	TaskID    string
	Title     string
	Stage     string
	Model     string
	Repos     []string
	Generated time.Time
}

var docKinds = map[string]string{"techdoc_rfc": "Technical design", "task_breakdown": "Task breakdown"}

// Frames reports whether stage's document is framed by Render.
func Frames(stage string) bool { return docKinds[stage] != "" }

// Render puts a title (unless the agent's content already opens with one) and a metadata line
// above the agent's content, which is kept verbatim.
func Render(m Meta, content string) string {
	var b strings.Builder
	body := strings.TrimLeft(content, "\n")
	if !strings.HasPrefix(body, "# ") {
		title := m.Title
		if title == "" {
			title = m.TaskID
		}
		fmt.Fprintf(&b, "# %s: %s\n\n", docKinds[m.Stage], title)
	} else if i := strings.Index(body, "\n"); i >= 0 {
		b.WriteString(body[:i+1] + "\n")
		body = strings.TrimLeft(body[i+1:], "\n")
	} else {
		b.WriteString(body + "\n\n")
		body = ""
	}
	parts := []string{"Task " + m.TaskID, "stage " + m.Stage}
	if len(m.Repos) > 0 {
		parts = append(parts, "repositories "+strings.Join(m.Repos, ", "))
	}
	if m.Model != "" {
		parts = append(parts, "written by "+m.Model)
	}
	if !m.Generated.IsZero() {
		parts = append(parts, m.Generated.Format("2006-01-02 15:04"))
	}
	fmt.Fprintf(&b, "> %s\n\n", strings.Join(parts, " · "))
	b.WriteString(body)
	return b.String()
}
