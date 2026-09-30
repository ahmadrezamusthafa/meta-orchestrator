// Package uat turns the UAT stage's structured test plan into a step-by-step manual testing
// guide with a screenshot of the real UI after every step.
package uat

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Step actions the screenshot runner can drive. "manual" steps are written into the guide but
// not automated (e.g. "check the invoice email"); "api" steps are requests an engineer sends.
const (
	ActionGoto       = "goto"
	ActionClick      = "click"
	ActionFill       = "fill"
	ActionSelect     = "select"
	ActionCheck      = "check"
	ActionPress      = "press"
	ActionWait       = "wait"
	ActionExpectText = "expect_text"
	ActionManual     = "manual"
	ActionAPI        = "api"
)

var knownActions = map[string]bool{ActionGoto: true, ActionClick: true, ActionFill: true, ActionSelect: true,
	ActionCheck: true, ActionPress: true, ActionWait: true, ActionExpectText: true, ActionManual: true, ActionAPI: true}

// Automated reports whether the runner drives the step in the browser.
func (s Step) Automated() bool { return s.Action != ActionManual && s.Action != ActionAPI }

// Step is one tester action.
type Step struct {
	Action      string `json:"action"`
	Target      string `json:"target,omitempty"` // URL/path for goto; selector otherwise (label=, placeholder=, testid=, text=, role=, CSS)
	Value       string `json:"value,omitempty"`  // text to type, option to pick, key to press, text to expect
	Where       string `json:"where,omitempty"`  // screen and menu path, e.g. "Subscription Backyard › Contracts › Create contract"
	Description string `json:"description"`      // what the tester does, in plain language
	Expected    string `json:"expected,omitempty"`
}

// Scenario is one acceptance scenario walked through in the UI.
type Scenario struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	App                 string   `json:"app,omitempty"`           // App.ID the scenario runs in
	Covers              []string `json:"covers,omitempty"`        // ATDD case IDs this scenario walks through
	ExecutableBy        string   `json:"executable_by,omitempty"` // "Finance/Ops", "Engineer" or "Engineer + Finance/Ops"
	Preconditions       []string `json:"preconditions,omitempty"`
	AcceptanceCriterion string   `json:"acceptance_criterion,omitempty"`
	Steps               []Step   `json:"steps"`
	ExpectedResult      string   `json:"expected_result,omitempty"` // the observable fact that passes the scenario
	Sanity              bool     `json:"sanity,omitempty"`          // run before the UAT scenarios; set from the covered cases
}

// Plan is the machine-readable UAT plan the UAT stage agent emits.
type Plan struct {
	Feature       string     `json:"feature"`
	Objective     string     `json:"objective,omitempty"`
	BaseURL       string     `json:"base_url,omitempty"`
	Preconditions []string   `json:"preconditions,omitempty"`
	TestData      []string   `json:"test_data,omitempty"` // descriptions only — never real customer data
	Scenarios     []Scenario `json:"scenarios"`
}

// PlanFence is the fenced code block language the agent wraps the plan in.
const PlanFence = "uat-plan"

var planBlock = regexp.MustCompile("(?s)```" + PlanFence + "\\s*\\n(.*?)\\n\\s*```")

// ExtractPlan finds and validates the ```uat-plan block in the stage output.
func ExtractPlan(content string) (*Plan, error) {
	m := planBlock.FindStringSubmatch(content)
	if m == nil {
		return nil, fmt.Errorf("the UAT stage output has no ```%s block", PlanFence)
	}
	var p Plan
	if err := json.Unmarshal([]byte(m[1]), &p); err != nil {
		return nil, fmt.Errorf("the ```%s block is not valid JSON: %w", PlanFence, err)
	}
	if err := p.normalize(); err != nil {
		return nil, err
	}
	return &p, nil
}

var safeID = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// normalize fills IDs and rejects plans the runner cannot execute.
func (p *Plan) normalize() error {
	if len(p.Scenarios) == 0 {
		return fmt.Errorf("the UAT plan has no scenarios")
	}
	seen := map[string]bool{}
	for i := range p.Scenarios {
		s := &p.Scenarios[i]
		s.ID = strings.Trim(safeID.ReplaceAllString(s.ID, "-"), "-")
		if s.ID == "" || seen[s.ID] {
			s.ID = fmt.Sprintf("S%d", i+1)
		}
		seen[s.ID] = true
		if len(s.Steps) == 0 {
			return fmt.Errorf("scenario %s has no steps", s.ID)
		}
		for j := range s.Covers {
			s.Covers[j] = strings.TrimSpace(s.Covers[j])
		}
		for j := range s.Steps {
			st := &s.Steps[j]
			st.Action = strings.ToLower(strings.TrimSpace(st.Action))
			if !knownActions[st.Action] {
				return fmt.Errorf("scenario %s step %d: unknown action %q", s.ID, j+1, st.Action)
			}
		}
	}
	return nil
}

// ValidURL reports whether u is an absolute http(s) URL.
func ValidURL(u string) bool {
	parsed, err := url.Parse(u)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

// Validate checks every navigation of a scenario stays on its app's environment, so a plan cannot
// send the browser (with the tester's session) to another site. baseURL maps a scenario to the
// environment it runs against; scenarios without one are not replayed and are skipped here.
func (p *Plan) Validate(baseURL func(Scenario) string) error {
	for _, s := range p.Scenarios {
		env := baseURL(s)
		if env == "" {
			continue
		}
		if !ValidURL(env) {
			return fmt.Errorf("the environment URL %q for scenario %s must be an absolute http(s) URL", env, s.ID)
		}
		base, _ := url.Parse(env)
		for j, st := range s.Steps {
			if st.Action != ActionGoto {
				continue
			}
			u, err := url.Parse(st.Target)
			if err != nil {
				return fmt.Errorf("scenario %s step %d: invalid URL %q", s.ID, j+1, st.Target)
			}
			if u.IsAbs() && !strings.EqualFold(u.Host, base.Host) {
				return fmt.Errorf("scenario %s step %d leaves the UAT environment (%s)", s.ID, j+1, u.Host)
			}
		}
	}
	return nil
}

// ScreenshotName is the file a step's screenshot is saved as.
func ScreenshotName(scenarioID string, step int) string {
	return fmt.Sprintf("%s-%02d.png", scenarioID, step)
}

// secretRef matches a ${UAT_…} placeholder the runner resolves from its environment.
var secretRef = regexp.MustCompile(`\$\{(UAT_[A-Z0-9_]+)\}`)
