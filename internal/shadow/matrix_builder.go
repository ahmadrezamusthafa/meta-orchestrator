package shadow

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// Matrix sources.
const (
	SourceShadowBenchmark = "shadow_benchmark"
	SourceDefault         = "default"
)

// ScoreWeights are w1..w3 of Score = w1·FPVR − w2·NormalizedCost − w3·NormalizedTTR.
type ScoreWeights struct {
	FPVR float64 `json:"fpvr"`
	Cost float64 `json:"cost"`
	TTR  float64 `json:"ttr"`
}

// Score evaluates the heuristic. fpvr is 0..1; normalized cost/TTR are 0..1 within a tuple.
func (w ScoreWeights) Score(fpvr, normCost, normTTR float64) float64 {
	return w.FPVR*fpvr - w.Cost*normCost - w.TTR*normTTR
}

// BuilderConfig tunes matrix compilation.
type BuilderConfig struct {
	Weights    ScoreWeights `json:"weights"`
	MinSamples int          `json:"min_samples"` // candidates with fewer samples cannot win
}

// DefaultBuilderConfig weights first-pass quality over cost and latency.
func DefaultBuilderConfig() BuilderConfig {
	return BuilderConfig{Weights: ScoreWeights{FPVR: 1.0, Cost: 0.3, TTR: 0.1}, MinSamples: 3}
}

// MatrixCell is one (stage, complexity) entry of configs/best_methods_matrix.json.
type MatrixCell struct {
	StageID       string   `json:"stage_id"`
	Complexity    string   `json:"complexity"`
	OptimalMethod string   `json:"optimal_method"`
	WinningModel  string   `json:"winning_model"`
	ModelTier     string   `json:"model_tier"`
	FPVRPercent   float64  `json:"fpvr_percent"`
	AvgTokens     float64  `json:"avg_tokens"`
	AvgDurationS  float64  `json:"avg_duration_s"`
	AvgCostUSD    float64  `json:"avg_cost_usd"`
	Score         float64  `json:"score"`
	Samples       int      `json:"samples"`
	ParetoFront   []string `json:"pareto_front,omitempty"` // "Method@model" candidates not dominated
	RunnerUp      string   `json:"runner_up,omitempty"`
	Source        string   `json:"source,omitempty"` // shadow_benchmark (measured) or default (policy)
}

// BestMethodsMatrix is the router-facing optimal method table.
type BestMethodsMatrix struct {
	Version     int          `json:"version"`
	GeneratedAt time.Time    `json:"generated_at"`
	Source      string       `json:"source"`
	SweepID     string       `json:"sweep_id,omitempty"`
	Weights     ScoreWeights `json:"weights"`
	Cells       []MatrixCell `json:"cells"`
}

// Cell returns the entry for a tuple.
func (m BestMethodsMatrix) Cell(stage, complexity string) (MatrixCell, bool) {
	for _, c := range m.Cells {
		if c.StageID == stage && c.Complexity == complexity {
			return c, true
		}
	}
	return MatrixCell{}, false
}

// MatrixBuilder compiles benchmark results into a BestMethodsMatrix.
type MatrixBuilder struct {
	cfg BuilderConfig
}

// NewMatrixBuilder creates a builder.
func NewMatrixBuilder(cfg BuilderConfig) *MatrixBuilder {
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 1
	}
	return &MatrixBuilder{cfg: cfg}
}

// Candidate is one scored (method, model) option of a tuple.
type Candidate struct {
	Result   CellResult `json:"result"`
	NormCost float64    `json:"norm_cost"`
	NormTTR  float64    `json:"norm_ttr"`
	Score    float64    `json:"score"`
	Pareto   bool       `json:"pareto"`
	Eligible bool       `json:"eligible"`
}

// Label is "Method@model".
func (c Candidate) Label() string { return c.Result.Cell.Method + "@" + c.Result.Cell.Model }

// Tuple is the scored candidate set of one (stage, complexity).
type Tuple struct {
	Stage      string      `json:"stage"`
	Complexity string      `json:"complexity"`
	Candidates []Candidate `json:"candidates"` // sorted by score descending; winner first if eligible
}

// Score groups results by tuple, normalizes cost and TTR within each tuple (min-max),
// computes the heuristic score and marks the Pareto-efficient set.
func (b *MatrixBuilder) Score(results []CellResult) []Tuple {
	groups := map[[2]string][]CellResult{}
	for _, r := range results {
		k := [2]string{r.Cell.Stage, r.Cell.Complexity}
		groups[k] = append(groups[k], r)
	}
	var tuples []Tuple
	for k, rs := range groups {
		t := Tuple{Stage: k[0], Complexity: k[1]}
		var eligible []CellResult
		for _, r := range rs {
			if r.Samples >= b.cfg.MinSamples {
				eligible = append(eligible, r)
			}
		}
		minC, maxC, minT, maxT := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, r := range eligible {
			minC, maxC = math.Min(minC, r.AvgCostUSD), math.Max(maxC, r.AvgCostUSD)
			minT, maxT = math.Min(minT, r.AvgTTRSeconds), math.Max(maxT, r.AvgTTRSeconds)
		}
		norm := func(v, lo, hi float64) float64 {
			if hi-lo <= 0 {
				return 0
			}
			return (v - lo) / (hi - lo)
		}
		for _, r := range rs {
			c := Candidate{Result: r, Eligible: r.Samples >= b.cfg.MinSamples}
			if c.Eligible {
				c.NormCost = norm(r.AvgCostUSD, minC, maxC)
				c.NormTTR = norm(r.AvgTTRSeconds, minT, maxT)
				c.Score = b.cfg.Weights.Score(r.FPVR, c.NormCost, c.NormTTR)
			}
			t.Candidates = append(t.Candidates, c)
		}
		for i := range t.Candidates {
			a := &t.Candidates[i]
			if !a.Eligible {
				continue
			}
			a.Pareto = true
			for _, o := range t.Candidates {
				if o.Eligible && dominates(o.Result, a.Result) {
					a.Pareto = false
					break
				}
			}
		}
		sort.SliceStable(t.Candidates, func(i, j int) bool {
			a, c := t.Candidates[i], t.Candidates[j]
			if a.Eligible != c.Eligible {
				return a.Eligible
			}
			if a.Pareto != c.Pareto {
				return a.Pareto
			}
			if a.Score != c.Score {
				return a.Score > c.Score
			}
			if a.Result.AvgCostUSD != c.Result.AvgCostUSD {
				return a.Result.AvgCostUSD < c.Result.AvgCostUSD
			}
			return a.Label() < c.Label()
		})
		tuples = append(tuples, t)
	}
	sort.Slice(tuples, func(i, j int) bool {
		if si, sj := stageIndex(tuples[i].Stage), stageIndex(tuples[j].Stage); si != sj {
			return si < sj
		}
		return complexityIndex(tuples[i].Complexity) < complexityIndex(tuples[j].Complexity)
	})
	return tuples
}

// dominates reports whether a is at least as good as b on FPVR, cost and TTR, and better on one.
func dominates(a, b CellResult) bool {
	if a.FPVR < b.FPVR || a.AvgCostUSD > b.AvgCostUSD || a.AvgTTRSeconds > b.AvgTTRSeconds {
		return false
	}
	return a.FPVR > b.FPVR || a.AvgCostUSD < b.AvgCostUSD || a.AvgTTRSeconds < b.AvgTTRSeconds
}

// Build selects the winning (method, model) per tuple: the highest-scoring Pareto-efficient
// candidate with enough samples. Tuples without an eligible candidate are omitted.
func (b *MatrixBuilder) Build(sweepID string, results []CellResult, now time.Time) BestMethodsMatrix {
	m := BestMethodsMatrix{Version: 1, GeneratedAt: now.UTC(), Source: SourceShadowBenchmark, SweepID: sweepID,
		Weights: b.cfg.Weights, Cells: []MatrixCell{}}
	for _, t := range b.Score(results) {
		if len(t.Candidates) == 0 || !t.Candidates[0].Eligible {
			continue
		}
		w := t.Candidates[0]
		cell := MatrixCell{StageID: t.Stage, Complexity: t.Complexity, OptimalMethod: w.Result.Cell.Method,
			WinningModel: w.Result.Cell.Model, ModelTier: w.Result.Cell.Tier, FPVRPercent: round1(w.Result.FPVR * 100),
			AvgTokens: math.Round(w.Result.AvgTokens), AvgDurationS: round1(w.Result.AvgTTRSeconds),
			AvgCostUSD: w.Result.AvgCostUSD, Score: math.Round(w.Score*1e4) / 1e4, Samples: w.Result.Samples,
			Source: SourceShadowBenchmark}
		for _, c := range t.Candidates {
			if c.Pareto {
				cell.ParetoFront = append(cell.ParetoFront, c.Label())
			}
		}
		if len(t.Candidates) > 1 && t.Candidates[1].Eligible {
			cell.RunnerUp = t.Candidates[1].Label()
		}
		m.Cells = append(m.Cells, cell)
	}
	return m
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// MergeMatrix overlays next onto prev: tuples next measured replace prev's, others are kept.
func MergeMatrix(prev, next BestMethodsMatrix) BestMethodsMatrix {
	out := next
	out.Cells = append([]MatrixCell(nil), next.Cells...)
	have := map[[2]string]bool{}
	for _, c := range next.Cells {
		have[[2]string{c.StageID, c.Complexity}] = true
	}
	for _, c := range prev.Cells {
		if !have[[2]string{c.StageID, c.Complexity}] {
			out.Cells = append(out.Cells, c)
		}
	}
	sortMatrixCells(out.Cells)
	return out
}

func sortMatrixCells(cells []MatrixCell) {
	sort.SliceStable(cells, func(i, j int) bool {
		if si, sj := stageIndex(cells[i].StageID), stageIndex(cells[j].StageID); si != sj {
			return si < sj
		}
		return complexityIndex(cells[i].Complexity) < complexityIndex(cells[j].Complexity)
	})
}

// Stages and Complexities are the canonical matrix axes.
var (
	Stages = []string{"prd_discovery", "repo_discovery", "atdd_creation", "techdoc_rfc", "task_breakdown",
		"red_verification", "task_implementation", "e2e_validation", "signoff_merge"}
	Complexities = []string{"LOW", "MEDIUM", "HIGH", "SYSTEM"}
)

func stageIndex(s string) int {
	for i, v := range Stages {
		if v == s {
			return i
		}
	}
	return len(Stages)
}

func complexityIndex(c string) int {
	for i, v := range Complexities {
		if v == c {
			return i
		}
	}
	return len(Complexities)
}

// DefaultMatrix encodes the best-practice policy for all 36 cells before any benchmark has run.
// Its source is "default", so the router ignores it and keeps its built-in heuristics.
func DefaultMatrix(now time.Time) BestMethodsMatrix {
	m := BestMethodsMatrix{Version: 1, GeneratedAt: now.UTC(), Source: SourceDefault, Weights: DefaultBuilderConfig().Weights}
	for _, st := range Stages {
		for _, cx := range Complexities {
			method, tier := "BMAD", "tier1"
			switch {
			case cx == "SYSTEM":
				method = "Superpower"
			case st == "repo_discovery" || st == "task_breakdown":
				method = "Supervisor"
			case st == "task_implementation" && cx == "HIGH":
				method = "Supervisor"
			case st == "task_implementation" || st == "red_verification":
				method, tier = "ReAct", "tier2"
			case st == "e2e_validation" || st == "signoff_merge":
				method, tier = "Superpower", "tier2"
			}
			m.Cells = append(m.Cells, MatrixCell{StageID: st, Complexity: cx, OptimalMethod: method, ModelTier: tier,
				Source: SourceDefault})
		}
	}
	return m
}

// FillDefaults returns m with every cell tagged by source and any of the 36 canonical cells
// it lacks taken from DefaultMatrix. It is for display only: the store keeps measured cells
// alone so the router never mistakes policy for evidence.
func FillDefaults(m BestMethodsMatrix, now time.Time) BestMethodsMatrix {
	out := m
	out.Cells = make([]MatrixCell, 0, len(Stages)*len(Complexities))
	have := map[[2]string]bool{}
	for _, c := range m.Cells {
		if c.Source == "" {
			c.Source = m.Source
		}
		if c.Source == "" {
			c.Source = SourceDefault
		}
		have[[2]string{c.StageID, c.Complexity}] = true
		out.Cells = append(out.Cells, c)
	}
	def := DefaultMatrix(now)
	for _, c := range def.Cells {
		if !have[[2]string{c.StageID, c.Complexity}] {
			out.Cells = append(out.Cells, c)
		}
	}
	if len(m.Cells) == 0 {
		out.GeneratedAt, out.Source, out.Weights = def.GeneratedAt, def.Source, def.Weights
	}
	sortMatrixCells(out.Cells)
	return out
}

// MeasuredCells counts cells backed by shadow benchmark samples.
func (m BestMethodsMatrix) MeasuredCells() int {
	n := 0
	for _, c := range m.Cells {
		if c.Source == SourceShadowBenchmark {
			n++
		}
	}
	return n
}

// WriteMatrixFile writes the matrix atomically (temp file + rename).
func WriteMatrixFile(path string, m BestMethodsMatrix) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".best_methods_matrix-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadMatrixFile reads a matrix file.
func LoadMatrixFile(path string) (BestMethodsMatrix, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return BestMethodsMatrix{}, err
	}
	var m BestMethodsMatrix
	if err := json.Unmarshal(data, &m); err != nil {
		return BestMethodsMatrix{}, fmt.Errorf("invalid best methods matrix %s: %w", path, err)
	}
	return m, nil
}

// MatrixStore holds the active matrix, persists updates, emits router.matrix.updated and
// implements router.MethodAdvisor so routing adopts new winners without a restart.
type MatrixStore struct {
	mu         sync.RWMutex
	path       string
	publish    telemetry.Publisher
	current    BestMethodsMatrix
	modTime    time.Time
	minSamples int
}

// NewMatrixStore loads path if it exists (a missing file yields an empty default).
func NewMatrixStore(path string, publish telemetry.Publisher) *MatrixStore {
	s := &MatrixStore{path: path, publish: publish, minSamples: DefaultBuilderConfig().MinSamples}
	if m, err := LoadMatrixFile(path); err == nil {
		s.current = m
		if st, err := os.Stat(path); err == nil {
			s.modTime = st.ModTime()
		}
	}
	return s
}

// Current returns the active matrix.
func (s *MatrixStore) Current() BestMethodsMatrix {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Update persists m, swaps it in and emits router.matrix.updated.
func (s *MatrixStore) Update(m BestMethodsMatrix, reportPath string) error {
	if s.path != "" {
		if err := WriteMatrixFile(s.path, m); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.current = m
	if st, err := os.Stat(s.path); err == nil {
		s.modTime = st.ModTime()
	}
	s.mu.Unlock()
	if s.publish != nil {
		s.publish(&types.OrchestratorEvent{
			Type:      types.EventRouterMatrixUpdated,
			Timestamp: time.Now(),
			Payload: map[string]interface{}{
				"generated_at": m.GeneratedAt, "cells": len(m.Cells), "source": m.Source,
				"sweep_id": m.SweepID, "report_path": reportPath,
			},
		})
	}
	return nil
}

// Reload re-reads the file when its modification time changed. It reports whether it swapped.
func (s *MatrixStore) Reload() (bool, error) {
	st, err := os.Stat(s.path)
	if err != nil {
		return false, err
	}
	s.mu.RLock()
	unchanged := st.ModTime().Equal(s.modTime)
	s.mu.RUnlock()
	if unchanged {
		return false, nil
	}
	m, err := LoadMatrixFile(s.path)
	if err != nil {
		return false, err
	}
	s.mu.Lock()
	s.current = m
	s.modTime = st.ModTime()
	s.mu.Unlock()
	if s.publish != nil {
		s.publish(&types.OrchestratorEvent{Type: types.EventRouterMatrixUpdated, Timestamp: time.Now(),
			Payload: map[string]interface{}{"generated_at": m.GeneratedAt, "cells": len(m.Cells), "source": m.Source}})
	}
	return true, nil
}

// Watch polls the matrix file for external edits until ctx is cancelled.
func (s *MatrixStore) Watch(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_, _ = s.Reload()
		}
	}
}

// routerStages maps router/FSM stage identifiers onto matrix stages.
var routerStages = map[string]string{
	"INTAKE_PRD": "prd_discovery", "PRD_DISCOVERY": "prd_discovery",
	"REPO_DISCOVERY": "repo_discovery",
	"ATDD_RED_PHASE": "atdd_creation", "MOCK_ATDD": "atdd_creation", "ATDD_CREATION": "atdd_creation",
	"TECH_DOC_RFC": "techdoc_rfc", "CONTRACT_SPEC": "techdoc_rfc", "TECHDOC_RFC": "techdoc_rfc",
	"TASK_BREAKDOWN":       "task_breakdown",
	"RED_VERIFICATION":     "red_verification",
	"IMPLEMENTATION_GREEN": "task_implementation", "PATCH_IMPLEMENTATION": "task_implementation",
	"CODEGEN_IMPLEMENT": "task_implementation", "TASK_IMPLEMENTATION": "task_implementation",
	"E2E_AUTOMATION": "e2e_validation", "UAT_EVIDENCE": "e2e_validation", "VERIFY_REGRESSION": "e2e_validation",
	"E2E_VALIDATION": "e2e_validation", "UAT_VERIFICATION": "e2e_validation",
	"CONTRACT_VERIFY": "signoff_merge", "SIGNOFF_MERGE": "signoff_merge",
}

// CanonicalStage maps a router stage id to a matrix stage id ("" when unknown).
func CanonicalStage(stageID string) string { return routerStages[strings.ToUpper(stageID)] }

// CanonicalComplexity maps router complexity labels onto matrix strata (CRITICAL → HIGH).
func CanonicalComplexity(c string) string {
	switch strings.ToUpper(c) {
	case "LOW":
		return "LOW"
	case "", "MEDIUM":
		return "MEDIUM"
	case "HIGH", "CRITICAL":
		return "HIGH"
	case "SYSTEM":
		return "SYSTEM"
	}
	return ""
}

// AdviseMethod implements router.MethodAdvisor using benchmark-sourced cells only.
func (s *MatrixStore) AdviseMethod(stageID, complexity string) (method, tier, reason string, ok bool) {
	stage, cx := CanonicalStage(stageID), CanonicalComplexity(complexity)
	if stage == "" || cx == "" {
		return "", "", "", false
	}
	s.mu.RLock()
	m := s.current
	s.mu.RUnlock()
	if m.Source != SourceShadowBenchmark {
		return "", "", "", false
	}
	c, found := m.Cell(stage, cx)
	if !found || c.Samples < s.minSamples || c.OptimalMethod == "" {
		return "", "", "", false
	}
	reason = fmt.Sprintf("Shadow benchmark %s: %s on %s wins %s/%s (FPVR %.0f%%, $%.4f, %d samples)",
		m.GeneratedAt.Format(time.RFC3339), c.OptimalMethod, c.WinningModel, stage, cx, c.FPVRPercent, c.AvgCostUSD, c.Samples)
	return strings.ToLower(c.OptimalMethod), c.ModelTier, reason, true
}
