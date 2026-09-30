package uat

import (
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
)

// CaseCoverage records where one in-scope ATDD case is walked through in the guide.
type CaseCoverage struct {
	Case      atdd.Case `json:"case"`
	App       string    `json:"app"`
	Scenarios []string  `json:"scenarios"`
	FromATDD  bool      `json:"from_atdd"` // the plan missed it; the guide uses the sheet's own steps
}

// Coverage is the UAT guide's contract with the ATDD sheet: every case marked UAT or Sanity is
// walked through, either by a plan scenario (with screenshots) or from the sheet's own steps.
type Coverage struct {
	Cases   []CaseCoverage `json:"cases"`
	Sanity  int            `json:"sanity"`
	UAT     int            `json:"uat"`
	Planned int            `json:"planned"` // covered by an agent-written scenario
	Missing []string       `json:"missing"` // case IDs the plan did not cover (filled from the sheet)
	Unknown []string       `json:"unknown"` // IDs the plan claims to cover that are not in the sheet
}

// Complete reports whether the plan itself covers every in-scope case.
func (c Coverage) Complete() bool { return len(c.Missing) == 0 }

func appExists(apps []App, id string) bool {
	for _, a := range apps {
		if a.ID == id {
			return true
		}
	}
	return false
}

// ApplyCoverage links the plan to the in-scope ATDD cases. It fills each scenario's app and
// sanity flag from the cases it covers, and appends a scenario built from the sheet for every
// case the plan left out, so the guide never silently drops a UAT or Sanity case.
func ApplyCoverage(p *Plan, scope []atdd.Case, apps []App) Coverage {
	var cov Coverage
	byID := map[string]atdd.Case{}
	for _, c := range scope {
		byID[strings.ToUpper(c.ID)] = c
	}
	covering := map[string][]string{}
	for i := range p.Scenarios {
		s := &p.Scenarios[i]
		var covered []atdd.Case
		for _, id := range s.Covers {
			c, ok := byID[strings.ToUpper(id)]
			if !ok {
				cov.Unknown = append(cov.Unknown, id)
				continue
			}
			covered = append(covered, c)
			covering[strings.ToUpper(c.ID)] = append(covering[strings.ToUpper(c.ID)], s.ID)
			s.Sanity = s.Sanity || c.Sanity
		}
		if !appExists(apps, s.App) {
			s.App = ""
			if len(covered) > 0 {
				s.App = AppFor(covered[0], apps)
			} else if len(apps) > 0 {
				s.App = AppFor(atdd.Case{Title: s.Title}, apps)
			}
		}
	}
	for _, c := range scope {
		if c.Sanity {
			cov.Sanity++
		}
		if c.UAT {
			cov.UAT++
		}
		cc := CaseCoverage{Case: c, Scenarios: covering[strings.ToUpper(c.ID)]}
		if len(cc.Scenarios) > 0 {
			cov.Planned++
			for _, s := range p.Scenarios {
				if s.ID == cc.Scenarios[0] {
					cc.App = s.App
				}
			}
		} else {
			cc.FromATDD = true
			cc.App = AppFor(c, apps)
			sc := scenarioFromCase(c, cc.App)
			p.Scenarios = append(p.Scenarios, sc)
			cc.Scenarios = []string{sc.ID}
			cov.Missing = append(cov.Missing, c.ID)
		}
		cov.Cases = append(cov.Cases, cc)
	}
	return cov
}

// scenarioFromCase turns a sheet row into a walkthrough: each step paired with the expected
// result of the same number when the sheet lines them up.
func scenarioFromCase(c atdd.Case, app string) Scenario {
	action := ActionManual
	if c.IsAPI() {
		action = ActionAPI
	}
	s := Scenario{ID: safeID.ReplaceAllString(c.ID, "-"), Title: c.Title, App: app, Covers: []string{c.ID},
		Preconditions: c.Preconditions, AcceptanceCriterion: c.AcceptanceCriteria, Sanity: c.Sanity}
	paired := len(c.ExpectedResults) == len(c.Steps)
	for i, st := range c.Steps {
		step := Step{Action: action, Description: st}
		if paired {
			step.Expected = c.ExpectedResults[i]
		}
		s.Steps = append(s.Steps, step)
	}
	if len(s.Steps) == 0 {
		s.Steps = []Step{{Action: action, Description: "Perform the check described in the title."}}
	}
	if !paired {
		s.ExpectedResult = strings.Join(c.ExpectedResults, " ")
	}
	if c.IsAPI() {
		s.ExecutableBy = "Engineer"
	}
	return s
}
