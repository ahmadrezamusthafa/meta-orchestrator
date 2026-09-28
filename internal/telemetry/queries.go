package telemetry

import (
	"fmt"
	"sort"
	"time"
)

// Totals is the headline aggregate of a summary window.
type Totals struct {
	Runs             int     `json:"runs"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	FPVRPercent      float64 `json:"fpvr_percent"`
	MTTRSeconds      float64 `json:"mttr_seconds"`
}

// CategoryStat is average token-per-feature per task category.
type CategoryStat struct {
	Category     string  `json:"category"`
	Runs         int     `json:"runs"`
	AvgTPFTokens float64 `json:"avg_tpf_tokens"`
	AvgCostUSD   float64 `json:"avg_cost_usd"`
	FPVRPercent  float64 `json:"fpvr_percent"`
}

// ModelStat is one model leaderboard row.
type ModelStat struct {
	Model          string  `json:"model"`
	Tier           string  `json:"tier"`
	TasksCompleted int     `json:"tasks_completed"`
	FPVRPercent    float64 `json:"fpvr_percent"`
	AvgTPFTokens   float64 `json:"avg_tpf_tokens"`
	AvgCostUSD     float64 `json:"avg_cost_usd"`
	AvgTTRSeconds  float64 `json:"avg_ttr_seconds"`
}

// MethodROI compares execution methodologies.
type MethodROI struct {
	Method        string  `json:"method"`
	Runs          int     `json:"runs"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	AvgCostUSD    float64 `json:"avg_cost_usd"`
	AvgTPFTokens  float64 `json:"avg_tpf_tokens"`
	FPVRPercent   float64 `json:"fpvr_percent"`
	AvgTTRSeconds float64 `json:"avg_ttr_seconds"`
}

// RepoStability is the ATDD stability index of a repository.
type RepoStability struct {
	Repo              string  `json:"repo"`
	Runs              int     `json:"runs"`
	FailureLoops      int     `json:"failure_loops"`
	AvgTestIterations float64 `json:"avg_test_iterations"`
	FlakinessIndex    float64 `json:"flakiness_index"` // failed test cycles / all test cycles, 0..1
}

// Summary is the /telemetry/summary response.
type Summary struct {
	Window         string          `json:"window"`
	Repo           string          `json:"repo"`
	GeneratedAt    time.Time       `json:"generated_at"`
	QueryLatencyMS float64         `json:"query_latency_ms"`
	Totals         Totals          `json:"totals"`
	ByCategory     []CategoryStat  `json:"by_category"`
	Leaderboard    []ModelStat     `json:"leaderboard"`
	Methods        []MethodROI     `json:"methods"`
	Stability      []RepoStability `json:"stability"`
	Repos          []string        `json:"repos"`
}

// BurnPoint is one token burn bucket.
type BurnPoint struct {
	Date             string  `json:"date"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	CachedTokens     int64   `json:"cached_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

// MTTRPoint is one rolling MTTR bucket.
type MTTRPoint struct {
	Date        string  `json:"date"`
	Runs        int     `json:"runs"`
	MTTRSeconds float64 `json:"mttr_seconds"`
	FPVRPercent float64 `json:"fpvr_percent"`
}

// Trends is the /telemetry/trends response.
type Trends struct {
	Window string      `json:"window"`
	Bucket string      `json:"bucket"`
	Burn   []BurnPoint `json:"burn"`
	MTTR   []MTTRPoint `json:"mttr"`
}

// Windows lists accepted window identifiers.
var Windows = map[string]time.Duration{
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
	"all": 0,
}

// ParseWindow validates a window identifier; empty defaults to 7d.
func ParseWindow(w string) (string, time.Duration, error) {
	if w == "" {
		w = "7d"
	}
	d, ok := Windows[w]
	if !ok {
		return "", 0, fmt.Errorf("invalid window %q (want 24h, 7d, 30d or all)", w)
	}
	return w, d, nil
}

func windowFilter(window string, repo string, now time.Time) Filter {
	f := Filter{Repo: repo}
	if d := Windows[window]; d > 0 {
		f.Since = now.Add(-d)
	}
	return f
}

var methodOrder = map[string]int{"BMAD": 0, "Supervisor": 1, "ReAct": 2, "Superpower": 3}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) * 100 / float64(d)
}

func avg(sum float64, n int) float64 {
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

type runAgg struct {
	runs, successes, firstPasses int
	tokens                       int64
	cost                         float64
	successDurUS                 int64
}

func (a *runAgg) add(r RunRecord) {
	a.runs++
	if r.Success {
		a.successes++
		a.successDurUS += r.DurationUS
	}
	if r.FirstPass {
		a.firstPasses++
	}
	a.tokens += r.TotalTokens()
	a.cost += r.CostUSD
}

// mttr is the mean TTR of successful (resolved) runs, in seconds.
func (a *runAgg) mttr() float64 { return avg(float64(a.successDurUS)/1e6, a.successes) }

// Summarize computes the aggregated summary for a window (24h, 7d, 30d, all) and optional repo.
func Summarize(ds *Datastore, window string, repo string, now time.Time) Summary {
	start := time.Now()
	f := windowFilter(window, repo, now)
	runs := ds.Runs(f)
	tokens := ds.Tokens(f)

	s := Summary{Window: window, Repo: repo, GeneratedAt: now,
		ByCategory: []CategoryStat{}, Leaderboard: []ModelStat{}, Methods: []MethodROI{}, Stability: []RepoStability{}, Repos: []string{}}

	for _, ev := range tokens {
		s.Totals.PromptTokens += ev.PromptTokens
		s.Totals.CompletionTokens += ev.CompletionTokens
		s.Totals.CachedTokens += ev.CachedTokens
		s.Totals.CostUSD += ev.CostUSD
	}
	s.Totals.TotalTokens = s.Totals.PromptTokens + s.Totals.CompletionTokens

	var all runAgg
	cats := map[string]*runAgg{}
	type modelKey struct{ model, tier string }
	models := map[modelKey]*runAgg{}
	methods := map[string]*runAgg{}
	type repoAgg struct {
		runs, loops, iters int
	}
	repos := map[string]*repoAgg{}

	for _, r := range runs {
		all.add(r)
		get := func(m map[string]*runAgg, k string) *runAgg {
			if m[k] == nil {
				m[k] = &runAgg{}
			}
			return m[k]
		}
		get(cats, r.Category).add(r)
		get(methods, r.Method).add(r)
		mk := modelKey{r.Model, r.Tier}
		if models[mk] == nil {
			models[mk] = &runAgg{}
		}
		models[mk].add(r)
		if r.Repo != "" {
			ra := repos[r.Repo]
			if ra == nil {
				ra = &repoAgg{}
				repos[r.Repo] = ra
			}
			ra.runs++
			ra.loops += r.FailureLoops
			ra.iters += r.TestIterations
		}
	}

	s.Totals.Runs = all.runs
	s.Totals.FPVRPercent = pct(all.firstPasses, all.runs)
	s.Totals.MTTRSeconds = all.mttr()
	// Without token rows (e.g. imported history), fall back to run-level token totals.
	if len(tokens) == 0 {
		for _, r := range runs {
			s.Totals.PromptTokens += r.PromptTokens
			s.Totals.CompletionTokens += r.CompletionTokens
			s.Totals.CachedTokens += r.CachedTokens
			s.Totals.CostUSD += r.CostUSD
		}
		s.Totals.TotalTokens = s.Totals.PromptTokens + s.Totals.CompletionTokens
	}

	for c, a := range cats {
		s.ByCategory = append(s.ByCategory, CategoryStat{Category: c, Runs: a.runs,
			AvgTPFTokens: avg(float64(a.tokens), a.runs), AvgCostUSD: avg(a.cost, a.runs), FPVRPercent: pct(a.firstPasses, a.runs)})
	}
	sort.Slice(s.ByCategory, func(i, j int) bool { return s.ByCategory[i].Category < s.ByCategory[j].Category })

	for k, a := range models {
		s.Leaderboard = append(s.Leaderboard, ModelStat{Model: k.model, Tier: k.tier, TasksCompleted: a.successes,
			FPVRPercent: pct(a.firstPasses, a.runs), AvgTPFTokens: avg(float64(a.tokens), a.runs),
			AvgCostUSD: avg(a.cost, a.runs), AvgTTRSeconds: a.mttr()})
	}
	sort.Slice(s.Leaderboard, func(i, j int) bool {
		a, b := s.Leaderboard[i], s.Leaderboard[j]
		if a.TasksCompleted != b.TasksCompleted {
			return a.TasksCompleted > b.TasksCompleted
		}
		if a.FPVRPercent != b.FPVRPercent {
			return a.FPVRPercent > b.FPVRPercent
		}
		return a.Model < b.Model
	})

	for m, a := range methods {
		s.Methods = append(s.Methods, MethodROI{Method: m, Runs: a.runs, TotalCostUSD: a.cost, AvgCostUSD: avg(a.cost, a.runs),
			AvgTPFTokens: avg(float64(a.tokens), a.runs), FPVRPercent: pct(a.firstPasses, a.runs), AvgTTRSeconds: a.mttr()})
	}
	sort.Slice(s.Methods, func(i, j int) bool {
		oi, iok := methodOrder[s.Methods[i].Method]
		oj, jok := methodOrder[s.Methods[j].Method]
		if iok != jok {
			return iok
		}
		if oi != oj {
			return oi < oj
		}
		return s.Methods[i].Method < s.Methods[j].Method
	})

	for name, a := range repos {
		st := RepoStability{Repo: name, Runs: a.runs, FailureLoops: a.loops, AvgTestIterations: avg(float64(a.iters), a.runs)}
		if a.iters > 0 {
			st.FlakinessIndex = float64(a.loops) / float64(a.iters)
		}
		s.Stability = append(s.Stability, st)
	}
	sort.Slice(s.Stability, func(i, j int) bool {
		if s.Stability[i].FlakinessIndex != s.Stability[j].FlakinessIndex {
			return s.Stability[i].FlakinessIndex > s.Stability[j].FlakinessIndex
		}
		return s.Stability[i].Repo < s.Stability[j].Repo
	})

	// The repo dropdown lists every repo in the window regardless of the active repo filter.
	seen := map[string]bool{}
	for _, r := range ds.Runs(windowFilter(window, "", now)) {
		if r.Repo != "" && !seen[r.Repo] {
			seen[r.Repo] = true
			s.Repos = append(s.Repos, r.Repo)
		}
	}
	sort.Strings(s.Repos)

	s.QueryLatencyMS = float64(time.Since(start).Microseconds()) / 1000
	return s
}

// ComputeTrends buckets token burn and MTTR over the window: hourly for 24h, daily otherwise.
func ComputeTrends(ds *Datastore, window string, repo string, now time.Time) Trends {
	f := windowFilter(window, repo, now)
	tokens := ds.Tokens(f)
	runs := ds.Runs(f)

	bucket, step, layout := "day", 24*time.Hour, "2006-01-02"
	if window == "24h" {
		bucket, step, layout = "hour", time.Hour, "2006-01-02T15:00"
	}
	truncate := func(t time.Time) time.Time {
		t = t.UTC()
		if bucket == "hour" {
			return t.Truncate(time.Hour)
		}
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	}

	end := truncate(now)
	var first time.Time
	if d := Windows[window]; d > 0 {
		first = end.Add(-d + step)
	} else {
		first = end
		if len(tokens) > 0 && truncate(tokens[0].Timestamp).Before(first) {
			first = truncate(tokens[0].Timestamp)
		}
		if len(runs) > 0 && truncate(runs[0].FinishedAt).Before(first) {
			first = truncate(runs[0].FinishedAt)
		}
		if limit := end.Add(-365 * 24 * time.Hour); first.Before(limit) {
			first = limit
		}
	}

	tr := Trends{Window: window, Bucket: bucket, Burn: []BurnPoint{}, MTTR: []MTTRPoint{}}
	index := map[string]int{}
	for t := first; !t.After(end); t = t.Add(step) {
		key := t.Format(layout)
		index[key] = len(tr.Burn)
		tr.Burn = append(tr.Burn, BurnPoint{Date: key})
		tr.MTTR = append(tr.MTTR, MTTRPoint{Date: key})
	}

	for _, ev := range tokens {
		if i, ok := index[truncate(ev.Timestamp).Format(layout)]; ok {
			b := &tr.Burn[i]
			b.PromptTokens += ev.PromptTokens
			b.CompletionTokens += ev.CompletionTokens
			b.CachedTokens += ev.CachedTokens
			b.CostUSD += ev.CostUSD
		}
	}
	aggs := make([]runAgg, len(tr.MTTR))
	for _, r := range runs {
		if i, ok := index[truncate(r.FinishedAt).Format(layout)]; ok {
			aggs[i].add(r)
		}
	}
	for i := range tr.MTTR {
		tr.MTTR[i].Runs = aggs[i].runs
		tr.MTTR[i].MTTRSeconds = aggs[i].mttr()
		tr.MTTR[i].FPVRPercent = pct(aggs[i].firstPasses, aggs[i].runs)
	}
	return tr
}
