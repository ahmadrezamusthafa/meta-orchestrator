// Package feedback turns historical run telemetry into routing heuristics and weights.
package feedback

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// CellKey identifies one cell of the success probability matrix:
// P(success | task_type, method, model_tier, router_strategy).
type CellKey struct {
	TaskType       string `json:"task_type"`
	Method         string `json:"method"`
	ModelTier      string `json:"model_tier"`
	RouterStrategy string `json:"router_strategy"`
}

// CellScore is the empirical performance of one cell.
type CellScore struct {
	CellKey
	Runs               int     `json:"runs"`
	Successes          int     `json:"successes"`
	FirstPasses        int     `json:"first_passes"`
	FrustrationHalts   int     `json:"frustration_halts"`
	FPVR               float64 `json:"fpvr"`                // first-pass verification rate, 0..1
	SuccessProbability float64 `json:"success_probability"` // Laplace-smoothed FPVR
	WilsonLower        float64 `json:"wilson_lower"`        // 95% lower confidence bound of FPVR
	AvgTPF             float64 `json:"avg_tpf"`             // average tokens per feature
	AvgCostUSD         float64 `json:"avg_cost_usd"`
	AvgTTRSeconds      float64 `json:"avg_ttr_seconds"`
}

// ProbabilityMatrix indexes cell scores.
type ProbabilityMatrix map[CellKey]CellScore

// Get returns a cell's score.
func (m ProbabilityMatrix) Get(taskType, method, tier, strategy string) (CellScore, bool) {
	c, ok := m[CellKey{TaskType: taskType, Method: method, ModelTier: NormalizeTier(tier), RouterStrategy: strategy}]
	return c, ok
}

// Probability returns P(success | cell), or the uninformed prior 0.5 for unseen cells.
func (m ProbabilityMatrix) Probability(taskType, method, tier, strategy string) float64 {
	if c, ok := m.Get(taskType, method, tier, strategy); ok {
		return c.SuccessProbability
	}
	return 0.5
}

// Cells returns all cells in deterministic order.
func (m ProbabilityMatrix) Cells() []CellScore {
	out := make([]CellScore, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i].CellKey, out[j].CellKey
		return strings.Join([]string{a.TaskType, a.Method, a.ModelTier, a.RouterStrategy}, "|") <
			strings.Join([]string{b.TaskType, b.Method, b.ModelTier, b.RouterStrategy}, "|")
	})
	return out
}

type accumulator struct {
	runs, successes, firstPasses, halts int
	tokens                              int64
	cost                                float64
	durUS                               int64
}

func (a *accumulator) add(r telemetry.RunRecord) {
	a.runs++
	if r.Success {
		a.successes++
	}
	if r.FirstPass {
		a.firstPasses++
	}
	if r.FrustrationHalt {
		a.halts++
	}
	a.tokens += r.TotalTokens()
	a.cost += r.CostUSD
	a.durUS += r.DurationUS
}

func (a *accumulator) fpvr() float64 {
	if a.runs == 0 {
		return 0
	}
	return float64(a.firstPasses) / float64(a.runs)
}

func (a *accumulator) avgCost() float64 {
	if a.runs == 0 {
		return 0
	}
	return a.cost / float64(a.runs)
}

func (a *accumulator) score(key CellKey) CellScore {
	n := float64(a.runs)
	c := CellScore{CellKey: key, Runs: a.runs, Successes: a.successes, FirstPasses: a.firstPasses, FrustrationHalts: a.halts}
	if a.runs == 0 {
		c.SuccessProbability = 0.5
		return c
	}
	c.FPVR = a.fpvr()
	c.SuccessProbability = (float64(a.firstPasses) + 1) / (n + 2)
	c.WilsonLower = wilsonLower(a.firstPasses, a.runs)
	c.AvgTPF = float64(a.tokens) / n
	c.AvgCostUSD = a.cost / n
	c.AvgTTRSeconds = float64(a.durUS) / 1e6 / n
	return c
}

// wilsonLower is the lower bound of the 95% Wilson score interval.
func wilsonLower(successes, n int) float64 {
	lower, _ := WilsonInterval(successes, n)
	return lower
}

// WilsonInterval returns the 95% Wilson score interval of a proportion.
func WilsonInterval(successes, n int) (lower, upper float64) {
	if n == 0 {
		return 0, 0
	}
	const z = 1.96
	p := float64(successes) / float64(n)
	nf := float64(n)
	denom := 1 + z*z/nf
	centre := p + z*z/(2*nf)
	margin := z * math.Sqrt(p*(1-p)/nf+z*z/(4*nf*nf))
	return (centre - margin) / denom, (centre + margin) / denom
}

// ScoreRuns computes the success probability matrix from run history.
func ScoreRuns(runs []telemetry.RunRecord) ProbabilityMatrix {
	acc := map[CellKey]*accumulator{}
	for _, r := range runs {
		k := CellKey{TaskType: r.Category, Method: r.Method, ModelTier: NormalizeTier(r.Tier), RouterStrategy: strategyOf(r)}
		if acc[k] == nil {
			acc[k] = &accumulator{}
		}
		acc[k].add(r)
	}
	m := make(ProbabilityMatrix, len(acc))
	for k, a := range acc {
		m[k] = a.score(k)
	}
	return m
}

func strategyOf(r telemetry.RunRecord) string {
	if r.RouterStrategy == "" {
		return "best_practice"
	}
	return strings.ToLower(r.RouterStrategy)
}

var tierDigits = regexp.MustCompile(`tier[\s_-]*([0-9]+)`)

// NormalizeTier maps "Tier 1", "TIER2", "tier_3", "tier2_codegen" to "tier1".."tier3".
func NormalizeTier(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	if m := tierDigits.FindStringSubmatch(s); m != nil {
		return "tier" + m[1]
	}
	return s
}

// tierRank orders tiers: tier1 (most capable, most expensive) = 1. Unknown tiers rank 0.
func tierRank(t string) int {
	if m := tierDigits.FindStringSubmatch(t); m != nil {
		n := 0
		for _, ch := range m[1] {
			n = n*10 + int(ch-'0')
		}
		return n
	}
	return 0
}
