package feedback

import (
	"math"
	"sort"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// Config tunes the heuristic evaluator.
type Config struct {
	// DowngradePassRate is the minimum FPVR a lower tier needs to absorb a task type (0.90 = 90%).
	DowngradePassRate float64 `json:"downgrade_pass_rate"`
	// MinSamples is the minimum runs a tier needs before it can be recommended.
	MinSamples int `json:"min_samples"`
	// MaxFrustrationRate is the tolerated share of runs that tripped the frustration breaker.
	MaxFrustrationRate float64 `json:"max_frustration_rate"`
	// MinComparisonSamples is the minimum runs on each side of a custom-vs-best-practice comparison.
	MinComparisonSamples int `json:"min_comparison_samples"`
	// FPVRMargin is the FPVR gap (0..1) that decides a comparison on quality alone.
	FPVRMargin float64 `json:"fpvr_margin"`
}

// DefaultConfig returns the plan's thresholds: ≥90% pass rate, no frustration loops.
func DefaultConfig() Config {
	return Config{DowngradePassRate: 0.90, MinSamples: 10, MaxFrustrationRate: 0, MinComparisonSamples: 5, FPVRMargin: 0.05}
}

// Winner labels the outcome of a custom rule vs best practice comparison.
type Winner string

const (
	WinnerCustom       Winner = "custom"
	WinnerBestPractice Winner = "best_practice"
	WinnerInconclusive Winner = "inconclusive"
)

// DowngradeCandidate is a task type a cheaper tier handles reliably.
type DowngradeCandidate struct {
	TaskType            string  `json:"task_type"`
	FromTier            string  `json:"from_tier"`
	ToTier              string  `json:"to_tier"`
	PassRate            float64 `json:"pass_rate"`
	Runs                int     `json:"runs"`
	FromAvgCostUSD      float64 `json:"from_avg_cost_usd"`
	ToAvgCostUSD        float64 `json:"to_avg_cost_usd"`
	EstimatedSavingsPct float64 `json:"estimated_savings_pct"`
}

// RuleComparison compares one custom rule to best-practice routing on the same task type.
type RuleComparison struct {
	RuleID                 string  `json:"rule_id"`
	TaskType               string  `json:"task_type"`
	CustomRuns             int     `json:"custom_runs"`
	BestPracticeRuns       int     `json:"best_practice_runs"`
	CustomFPVR             float64 `json:"custom_fpvr"`
	BestPracticeFPVR       float64 `json:"best_practice_fpvr"`
	CustomAvgCostUSD       float64 `json:"custom_avg_cost_usd"`
	BestPracticeAvgCostUSD float64 `json:"best_practice_avg_cost_usd"`
	Winner                 Winner  `json:"winner"`
}

// Report is the evaluator output.
type Report struct {
	Matrix              ProbabilityMatrix    `json:"-"`
	Cells               []CellScore          `json:"cells"`
	Downgrades          []DowngradeCandidate `json:"downgrades"`
	RuleComparisons     []RuleComparison     `json:"rule_comparisons"`
	HighPerformingRules []string             `json:"high_performing_rules"`
}

// Evaluator derives routing heuristics from run history.
type Evaluator struct {
	cfg Config
}

// NewEvaluator creates an evaluator.
func NewEvaluator(cfg Config) *Evaluator {
	return &Evaluator{cfg: cfg}
}

// Evaluate scores the history, flags tier downgrades and compares custom rules with best practice.
func (e *Evaluator) Evaluate(runs []telemetry.RunRecord) Report {
	m := ScoreRuns(runs)
	rep := Report{Matrix: m, Cells: m.Cells(), Downgrades: []DowngradeCandidate{}, RuleComparisons: []RuleComparison{}, HighPerformingRules: []string{}}
	rep.Downgrades = e.downgrades(runs)
	rep.RuleComparisons = e.compareRules(runs)
	for _, c := range rep.RuleComparisons {
		if c.Winner == WinnerCustom {
			rep.HighPerformingRules = append(rep.HighPerformingRules, c.RuleID)
		}
	}
	return rep
}

// downgrades finds, per task type, the cheapest tier below the most capable tier observed that
// meets the pass-rate bar with enough samples and without frustration loops.
func (e *Evaluator) downgrades(runs []telemetry.RunRecord) []DowngradeCandidate {
	byType := map[string]map[string]*accumulator{}
	for _, r := range runs {
		tier := NormalizeTier(r.Tier)
		if tierRank(tier) == 0 {
			continue
		}
		if byType[r.Category] == nil {
			byType[r.Category] = map[string]*accumulator{}
		}
		if byType[r.Category][tier] == nil {
			byType[r.Category][tier] = &accumulator{}
		}
		byType[r.Category][tier].add(r)
	}

	out := []DowngradeCandidate{}
	for taskType, tiers := range byType {
		from := ""
		for t := range tiers {
			if from == "" || tierRank(t) < tierRank(from) {
				from = t
			}
		}
		var best *DowngradeCandidate
		for t, a := range tiers {
			if tierRank(t) <= tierRank(from) || a.runs < e.cfg.MinSamples {
				continue
			}
			if a.fpvr() < e.cfg.DowngradePassRate {
				continue
			}
			if float64(a.halts)/float64(a.runs) > e.cfg.MaxFrustrationRate {
				continue
			}
			fromCost := tiers[from].avgCost()
			savings := 0.0
			if fromCost > 0 {
				savings = (fromCost - a.avgCost()) / fromCost * 100
			}
			cand := DowngradeCandidate{TaskType: taskType, FromTier: from, ToTier: t, PassRate: a.fpvr(), Runs: a.runs,
				FromAvgCostUSD: fromCost, ToAvgCostUSD: a.avgCost(), EstimatedSavingsPct: savings}
			// Prefer the lowest (cheapest) qualifying tier.
			if best == nil || tierRank(t) > tierRank(best.ToTier) {
				c := cand
				best = &c
			}
		}
		if best != nil {
			out = append(out, *best)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TaskType < out[j].TaskType })
	return out
}

func (e *Evaluator) compareRules(runs []telemetry.RunRecord) []RuleComparison {
	type ruleKey struct{ rule, taskType string }
	custom := map[ruleKey]*accumulator{}
	bp := map[string]*accumulator{}
	for _, r := range runs {
		switch strategyOf(r) {
		case "custom":
			if r.RuleID == "" {
				continue
			}
			k := ruleKey{r.RuleID, r.Category}
			if custom[k] == nil {
				custom[k] = &accumulator{}
			}
			custom[k].add(r)
		case "best_practice":
			if bp[r.Category] == nil {
				bp[r.Category] = &accumulator{}
			}
			bp[r.Category].add(r)
		}
	}

	out := []RuleComparison{}
	for k, c := range custom {
		b := bp[k.taskType]
		if b == nil {
			b = &accumulator{}
		}
		cmp := RuleComparison{RuleID: k.rule, TaskType: k.taskType, CustomRuns: c.runs, BestPracticeRuns: b.runs,
			CustomFPVR: c.fpvr(), BestPracticeFPVR: b.fpvr(), CustomAvgCostUSD: c.avgCost(), BestPracticeAvgCostUSD: b.avgCost(),
			Winner: WinnerInconclusive}
		if c.runs >= e.cfg.MinComparisonSamples && b.runs >= e.cfg.MinComparisonSamples {
			cmp.Winner = decide(c, b, e.cfg.FPVRMargin)
		}
		out = append(out, cmp)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].TaskType != out[j].TaskType {
			return out[i].TaskType < out[j].TaskType
		}
		return out[i].RuleID < out[j].RuleID
	})
	return out
}

// decide picks a winner. An FPVR lead wins outright only when it is at least margin and
// statistically significant (one-sided two-proportion z-test, 95%); a lead that could be noise (4/5 vs
// 3/5) decides nothing. A side whose quality is not worse wins on lower cost. A significant
// quality loss is never offset by cost.
func decide(custom, bp *accumulator, margin float64) Winner {
	const eps = 1e-9
	cf, bf := custom.fpvr(), bp.fpvr()
	significant := significantDiff(custom.firstPasses, custom.runs, bp.firstPasses, bp.runs)
	cc, bc := custom.avgCost(), bp.avgCost()
	switch {
	case cf-bf >= margin-eps && significant:
		return WinnerCustom
	case bf-cf >= margin-eps && significant:
		return WinnerBestPractice
	case significant:
		return WinnerInconclusive
	case cf >= bf && cc < bc:
		return WinnerCustom
	case bf >= cf && bc < cc:
		return WinnerBestPractice
	default:
		return WinnerInconclusive
	}
}

// significantDiff runs a pooled two-proportion z-test, one-sided at the 95% level: the
// question is whether the side that leads is genuinely better.
func significantDiff(s1, n1, s2, n2 int) bool {
	if n1 == 0 || n2 == 0 {
		return false
	}
	p1, p2 := float64(s1)/float64(n1), float64(s2)/float64(n2)
	p := float64(s1+s2) / float64(n1+n2)
	se := math.Sqrt(p * (1 - p) * (1/float64(n1) + 1/float64(n2)))
	if se == 0 {
		return false // both sides all-pass or all-fail: no difference
	}
	return math.Abs(p1-p2)/se >= 1.645
}
