package atdd

import (
	"strings"
	"testing"
)

// Sheets differ in header position, column order and naming; the parser keys off header names.
const sheet19 = `,,,,,PRD,https://x/prd
,,,,,JIRA Epic,https://x/epic
,,,,,Tech Doc,
Case ID,Requirement,Reference,Priority,Test Level,Sanity,Title,Preconditions,Steps,Expected Results,Reference Acceptance Criteria,Tags,Platform,AI Generated,UAT,EM,QA,PM,Note
T-TC-001,US-1,US-1,P1,SIT,Yes,Menu appears,"I'm signed in.","1. Open Backyard.
2. Open the sidebar.","1. Menu shown.
2. Opens list.","US-1 [Scenario: Menu]",Functional|UI|Smoke,WEB,True,Yes,0,0,1,
T-TC-002,US-2,US-2,P2,SUT,,API contract,,1. POST /x,1. 201,,Functional|API,API,True,,0,0,1,
T-TC-003,US-3,US-3,P0,SUT,Yes,Health endpoint,,1. GET /health,1. 200,,Functional|API,API,True,,0,0,1,note
`

const sheetCombined = `PRD,https://x
Case ID,Pokayoke Id,Requirement,Reference,Priority,Test Level,UAT/Sanity,Title,Preconditions,Steps,Expected Results,Reference Acceptance Criteria,Tags,Platform,Repository,AI Generated,EM,QA,PM,Note,Note
GT-1,MIB-1,US-3,US-3,P1,SIT,"UAT, Sanity",Send tags,,1. Save the SO,1. Tags sent,,,WEB,billing-frontend,True,0,0,1,first,second
GT-2,MIB-2,US-4,US-4,P2,SUT,,Internal only,,1. x,1. y,,,API,billing-dashboard,True,0,0,1,,
`

func TestParseCSVVariants(t *testing.T) {
	cases, err := ParseCSV(strings.NewReader(sheet19))
	if err != nil || len(cases) != 3 {
		t.Fatalf("19-column sheet → %d cases, %v", len(cases), err)
	}
	c := cases[0]
	if !c.UAT || !c.Sanity || c.Kind() != "Sanity + UAT" || len(c.Steps) != 2 || c.Steps[0] != "Open Backyard." || c.ExpectedResults[1] != "Opens list." {
		t.Fatalf("case 1 = %+v", c)
	}
	scope := UATScope(cases)
	if len(scope) != 2 || scope[1].ID != "T-TC-003" || !scope[1].IsAPI() || scope[1].Kind() != "Sanity" {
		t.Fatalf("scope = %+v", scope)
	}

	cases, err = ParseCSV(strings.NewReader(sheetCombined))
	if err != nil || len(cases) != 2 {
		t.Fatalf("combined sheet → %d, %v", len(cases), err)
	}
	if !cases[0].UAT || !cases[0].Sanity || cases[0].Repository != "billing-frontend" || cases[0].Note != "first" {
		t.Fatalf("combined case = %+v", cases[0])
	}
	if cases[1].InUATScope() {
		t.Fatal("blank UAT/Sanity is out of scope")
	}
	if _, err := ParseCSV(strings.NewReader("a,b\n1,2\n")); err == nil {
		t.Fatal("a sheet without a Case ID header must be rejected")
	}
}

func TestParseBlock(t *testing.T) {
	out := "cases\n\n```atdd-cases\n" + `{"cases":[{"id":"A-1","title":"t","uat":true,"sanity":"Yes","steps":["1. a","2. b"],"expected_results":"1. x\n2. y","platform":"WEB"},{"title":"no id"}]}` + "\n```\n"
	cases, err := ParseBlock(out)
	if err != nil || len(cases) != 1 {
		t.Fatalf("block → %+v, %v", cases, err)
	}
	if c := cases[0]; !c.UAT || !c.Sanity || len(c.Steps) != 2 || len(c.ExpectedResults) != 2 || c.ExpectedResults[1] != "y" {
		t.Fatalf("case = %+v", c)
	}
	if _, err := ParseBlock("nothing"); err == nil {
		t.Fatal("missing block should fail")
	}
}
