package atdd

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// Case is one acceptance test case from an ATDD sheet (the 19-column bmad-atdd CSV and the
// variants teams derive from it) or from the ATDD stage's ```atdd-cases block.
type Case struct {
	ID                 string   `json:"id"`
	Requirement        string   `json:"requirement,omitempty"`
	Reference          string   `json:"reference,omitempty"`
	Priority           string   `json:"priority,omitempty"`
	TestLevel          string   `json:"test_level,omitempty"`
	Title              string   `json:"title"`
	Preconditions      []string `json:"preconditions,omitempty"`
	Steps              []string `json:"steps,omitempty"`
	ExpectedResults    []string `json:"expected_results,omitempty"`
	AcceptanceCriteria string   `json:"reference_acceptance_criteria,omitempty"`
	Tags               string   `json:"tags,omitempty"`
	Platform           string   `json:"platform,omitempty"`   // WEB or API
	Repository         string   `json:"repository,omitempty"` // app/repo the case runs against, when the sheet says
	Note               string   `json:"note,omitempty"`
	UAT                bool     `json:"uat"`
	Sanity             bool     `json:"sanity"`
}

// InUATScope reports whether the case must appear in the UAT guide.
func (c Case) InUATScope() bool { return c.UAT || c.Sanity }

// IsAPI reports an operation-level case with no screen to drive.
func (c Case) IsAPI() bool { return strings.EqualFold(strings.TrimSpace(c.Platform), "API") }

// Kind labels the case for testers: "Sanity", "UAT" or "Sanity + UAT".
func (c Case) Kind() string {
	switch {
	case c.UAT && c.Sanity:
		return "Sanity + UAT"
	case c.Sanity:
		return "Sanity"
	case c.UAT:
		return "UAT"
	}
	return ""
}

func truthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "yes", "y", "true", "x", "1", "✓", "v":
		return true
	}
	return false
}

// columnAliases maps normalized header names to Case fields. Sheets reorder columns, rename
// a few, and sometimes merge UAT and Sanity into one column.
var columnAliases = map[string]string{
	"case id": "id", "requirement": "requirement", "reference": "reference", "priority": "priority",
	"test level": "test_level", "title": "title", "preconditions": "preconditions", "steps": "steps",
	"expected results": "expected", "expected result": "expected",
	"reference acceptance criteria": "ac", "acceptance criteria (prd)": "ac", "acceptance criteria": "ac",
	"tags": "tags", "platform": "platform", "repository": "repository", "repo": "repository", "note": "note",
	"uat": "uat", "sanity": "sanity", "uat/sanity": "uat_sanity", "uat / sanity": "uat_sanity",
}

// ParseCSV reads an ATDD sheet. Metadata rows before the "Case ID" header are skipped.
func ParseCSV(r io.Reader) ([]Case, error) {
	rd := csv.NewReader(r)
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	rows, err := rd.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read ATDD CSV: %w", err)
	}
	hdr := -1
	for i, row := range rows {
		if len(row) > 0 && strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(row[0], "\ufeff")), "Case ID") {
			hdr = i
			break
		}
	}
	if hdr < 0 {
		return nil, fmt.Errorf("not an ATDD sheet: no \"Case ID\" header row")
	}
	cols := map[string]int{}
	for i, h := range rows[hdr] {
		key := columnAliases[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(h, "\ufeff")))]
		if _, dup := cols[key]; key != "" && !dup { // first "Note" wins when a sheet repeats it
			cols[key] = i
		}
	}
	cell := func(row []string, key string) string {
		if i, ok := cols[key]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var out []Case
	for _, row := range rows[hdr+1:] {
		id := cell(row, "id")
		if id == "" {
			continue
		}
		c := Case{ID: id, Requirement: cell(row, "requirement"), Reference: cell(row, "reference"), Priority: cell(row, "priority"),
			TestLevel: cell(row, "test_level"), Title: cell(row, "title"), Preconditions: Lines(cell(row, "preconditions")),
			Steps: Lines(cell(row, "steps")), ExpectedResults: Lines(cell(row, "expected")), AcceptanceCriteria: cell(row, "ac"),
			Tags: cell(row, "tags"), Platform: cell(row, "platform"), Repository: cell(row, "repository"), Note: cell(row, "note"),
			UAT: truthy(cell(row, "uat")), Sanity: truthy(cell(row, "sanity"))}
		if combined := strings.ToLower(cell(row, "uat_sanity")); combined != "" {
			c.UAT = c.UAT || strings.Contains(combined, "uat") || truthy(combined)
			c.Sanity = c.Sanity || strings.Contains(combined, "sanity")
		}
		out = append(out, c)
	}
	return out, nil
}

var numbered = regexp.MustCompile(`^\s*(\d+[.)]|[-*•])\s+`)

// Lines splits a multi-line cell into items, dropping "1." / "-" markers.
func Lines(cell string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(cell, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(numbered.ReplaceAllString(l, "")); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// CasesFence is the fenced block language the ATDD stage emits its cases in.
const CasesFence = "atdd-cases"

var casesBlock = regexp.MustCompile("(?s)```" + CasesFence + "\\s*\\n(.*?)\\n\\s*```")

// ParseBlock reads the ```atdd-cases JSON block from the ATDD stage output. It accepts either a
// bare array or {"cases": [...]}; uat/sanity may be booleans or "Yes".
func ParseBlock(content string) ([]Case, error) {
	m := casesBlock.FindStringSubmatch(content)
	if m == nil {
		return nil, fmt.Errorf("no ```%s block", CasesFence)
	}
	raw := strings.TrimSpace(m[1])
	if strings.HasPrefix(raw, "{") {
		var wrap struct {
			Cases json.RawMessage `json:"cases"`
		}
		if err := json.Unmarshal([]byte(raw), &wrap); err != nil {
			return nil, fmt.Errorf("the ```%s block is not valid JSON: %w", CasesFence, err)
		}
		raw = string(wrap.Cases)
	}
	var loose []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &loose); err != nil {
		return nil, fmt.Errorf("the ```%s block is not valid JSON: %w", CasesFence, err)
	}
	str := func(v interface{}) string {
		switch x := v.(type) {
		case string:
			return strings.TrimSpace(x)
		case bool:
			if x {
				return "yes"
			}
			return ""
		case nil:
			return ""
		}
		return fmt.Sprint(v)
	}
	list := func(v interface{}) []string {
		if arr, ok := v.([]interface{}); ok {
			var out []string
			for _, x := range arr {
				out = append(out, Lines(str(x))...)
			}
			return out
		}
		return Lines(str(v))
	}
	var out []Case
	for _, c := range loose {
		id := str(c["id"])
		if id == "" {
			continue
		}
		out = append(out, Case{ID: id, Requirement: str(c["requirement"]), Reference: str(c["reference"]), Priority: str(c["priority"]),
			TestLevel: str(c["test_level"]), Title: str(c["title"]), Preconditions: list(c["preconditions"]), Steps: list(c["steps"]),
			ExpectedResults: list(c["expected_results"]), AcceptanceCriteria: str(c["reference_acceptance_criteria"]),
			Tags: str(c["tags"]), Platform: str(c["platform"]), Repository: str(c["repository"]), Note: str(c["note"]),
			UAT: truthy(str(c["uat"])), Sanity: truthy(str(c["sanity"]))})
	}
	return out, nil
}

// UATScope returns the cases the UAT guide must cover, in sheet order.
func UATScope(cases []Case) []Case {
	var out []Case
	for _, c := range cases {
		if c.InUATScope() {
			out = append(out, c)
		}
	}
	return out
}
