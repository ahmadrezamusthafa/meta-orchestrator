package feedback

import (
	"fmt"
	"math"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

type runSpec struct {
	category, method, tier, strategy, rule string
	n, firstPasses, halts                  int
	cost                                   float64
	tokens                                 int64
}

func build(specs ...runSpec) []telemetry.RunRecord {
	var out []telemetry.RunRecord
	id := 0
	for _, s := range specs {
		for i := 0; i < s.n; i++ {
			id++
			out = append(out, telemetry.RunRecord{
				RunMeta: telemetry.RunMeta{TaskID: fmt.Sprintf("H-%d", id), Category: s.category, Method: s.method,
					Tier: s.tier, RouterStrategy: s.strategy, RuleID: s.rule},
				Success:         i < s.firstPasses || i%2 == 0,
				FirstPass:       i < s.firstPasses,
				FrustrationHalt: i < s.halts,
				PromptTokens:    s.tokens, CostUSD: s.cost, DurationUS: 60e6,
			})
		}
	}
	return out
}

// historicalDataset models: routine CRUD passes on tier2 (downgrade candidate), migrations fail on
// tier2 (must stay tier1), docs pass on tier3 but once hit a frustration loop (not a candidate),
// and a project custom rule for CRUD that beats best practice.
func historicalDataset() []telemetry.RunRecord {
	return build(
		runSpec{category: "crud", method: "ReAct", tier: "tier1", strategy: "best_practice", n: 20, firstPasses: 19, cost: 0.20, tokens: 20000},
		runSpec{category: "crud", method: "ReAct", tier: "tier2", strategy: "best_practice", n: 20, firstPasses: 19, cost: 0.03, tokens: 9000},
		runSpec{category: "migration", method: "Supervisor", tier: "tier1", strategy: "best_practice", n: 15, firstPasses: 13, cost: 0.60, tokens: 60000},
		runSpec{category: "migration", method: "Supervisor", tier: "tier2", strategy: "best_practice", n: 15, firstPasses: 6, cost: 0.10, tokens: 30000},
		runSpec{category: "docs", method: "ReAct", tier: "tier3", strategy: "best_practice", n: 12, firstPasses: 12, halts: 1, cost: 0.001, tokens: 3000},
		runSpec{category: "crud", method: "BMAD", tier: "tier2", strategy: "custom", rule: "rule-crud-bmad", n: 22, firstPasses: 22, cost: 0.025, tokens: 8000},
		runSpec{category: "migration", method: "ReAct", tier: "tier2", strategy: "custom", rule: "rule-mig-react", n: 10, firstPasses: 3, cost: 0.05, tokens: 12000},
	)
}

func TestScoreRunsProbabilityMatrix(t *testing.T) {
	m := ScoreRuns(historicalDataset())
	c, ok := m.Get("crud", "ReAct", "tier2", "best_practice")
	if !ok {
		t.Fatal("missing crud/ReAct/tier2/best_practice cell")
	}
	if c.Runs != 20 || c.FirstPasses != 19 || math.Abs(c.FPVR-0.95) > 1e-12 {
		t.Fatalf("cell = %+v", c)
	}
	// P(success) is Laplace-smoothed: (19+1)/(20+2).
	if math.Abs(c.SuccessProbability-20.0/22) > 1e-12 {
		t.Fatalf("P(success) = %v", c.SuccessProbability)
	}
	if c.AvgTPF != 9000 || math.Abs(c.AvgCostUSD-0.03) > 1e-12 {
		t.Fatalf("tpf/cost = %v/%v", c.AvgTPF, c.AvgCostUSD)
	}
	if c.WilsonLower >= c.FPVR || c.WilsonLower <= 0.7 {
		t.Fatalf("wilson lower bound = %v", c.WilsonLower)
	}
	if _, ok := m.Get("crud", "ReAct", "tier9", "best_practice"); ok {
		t.Fatal("unexpected cell")
	}
	// Unseen cell probability falls back to the uninformed prior 0.5.
	if p := m.Probability("nothing", "ReAct", "tier2", "best_practice"); p != 0.5 {
		t.Fatalf("prior = %v", p)
	}
}

func TestEvaluatorClassifiesRoutineTasksForTierDowngrade(t *testing.T) {
	rep := NewEvaluator(DefaultConfig()).Evaluate(historicalDataset())

	byCat := map[string]DowngradeCandidate{}
	for _, d := range rep.Downgrades {
		byCat[d.TaskType] = d
	}
	crud, ok := byCat["crud"]
	if !ok {
		t.Fatalf("crud should be a downgrade candidate; got %+v", rep.Downgrades)
	}
	if crud.FromTier != "tier1" || crud.ToTier != "tier2" || crud.PassRate < 0.90 {
		t.Fatalf("crud candidate = %+v", crud)
	}
	if crud.EstimatedSavingsPct < 80 || crud.EstimatedSavingsPct > 90 {
		t.Fatalf("savings = %v, want ~85%% (0.20 → 0.03)", crud.EstimatedSavingsPct)
	}
	if _, ok := byCat["migration"]; ok {
		t.Fatal("migration must not be downgraded (tier2 FPVR 40%)")
	}
	if _, ok := byCat["docs"]; ok {
		t.Fatal("docs hit a frustration loop and must not be downgraded")
	}
}

func TestEvaluatorHighlightsCustomRulePerformance(t *testing.T) {
	rep := NewEvaluator(DefaultConfig()).Evaluate(historicalDataset())
	byRule := map[string]RuleComparison{}
	for _, c := range rep.RuleComparisons {
		byRule[c.RuleID] = c
	}
	crud := byRule["rule-crud-bmad"]
	if crud.Winner != WinnerCustom || crud.TaskType != "crud" || crud.CustomRuns != 22 {
		t.Fatalf("crud rule comparison = %+v", crud)
	}
	if crud.CustomFPVR != 1.0 || math.Abs(crud.BestPracticeFPVR-38.0/40) > 1e-12 {
		t.Fatalf("fpvr custom=%v bp=%v", crud.CustomFPVR, crud.BestPracticeFPVR)
	}
	mig := byRule["rule-mig-react"]
	if mig.Winner != WinnerBestPractice {
		t.Fatalf("migration custom rule should lose to best practice: %+v", mig)
	}
	if len(rep.HighPerformingRules) != 1 || rep.HighPerformingRules[0] != "rule-crud-bmad" {
		t.Fatalf("high performing rules = %v", rep.HighPerformingRules)
	}
}

func TestEvaluatorRequiresMinimumSamples(t *testing.T) {
	few := build(
		runSpec{category: "crud", method: "ReAct", tier: "tier1", strategy: "best_practice", n: 3, firstPasses: 3, cost: 0.2},
		runSpec{category: "crud", method: "ReAct", tier: "tier2", strategy: "best_practice", n: 3, firstPasses: 3, cost: 0.02},
		runSpec{category: "crud", method: "BMAD", tier: "tier2", strategy: "custom", rule: "r", n: 2, firstPasses: 2, cost: 0.01},
	)
	rep := NewEvaluator(DefaultConfig()).Evaluate(few)
	if len(rep.Downgrades) != 0 {
		t.Fatalf("downgrade with 3 samples: %+v", rep.Downgrades)
	}
	if len(rep.RuleComparisons) != 1 || rep.RuleComparisons[0].Winner != WinnerInconclusive {
		t.Fatalf("rule comparison = %+v", rep.RuleComparisons)
	}
}

func TestNormalizeTier(t *testing.T) {
	for in, want := range map[string]string{"Tier 1": "tier1", "TIER2": "tier2", "tier_3": "tier3", "tier2_codegen": "tier2", "": ""} {
		if got := NormalizeTier(in); got != want {
			t.Fatalf("NormalizeTier(%q) = %q, want %q", in, got, want)
		}
	}
}

// A small-sample FPVR gap is noise, not a verdict: 4/5 vs 3/5 clears the 5-point margin but
// not the significance test, and the costlier side must not win on quality alone.
func TestEvaluatorRuleComparisonIgnoresNoisyLead(t *testing.T) {
	runs := build(
		runSpec{category: "crud", method: "BMAD", tier: "tier1", strategy: "custom", rule: "r", n: 5, firstPasses: 4, cost: 0.30},
		runSpec{category: "crud", method: "ReAct", tier: "tier1", strategy: "best_practice", n: 5, firstPasses: 3, cost: 0.10},
	)
	rep := NewEvaluator(DefaultConfig()).Evaluate(runs)
	if len(rep.RuleComparisons) != 1 || rep.RuleComparisons[0].Winner != WinnerInconclusive {
		t.Fatalf("rule comparison = %+v, want inconclusive", rep.RuleComparisons)
	}
}
