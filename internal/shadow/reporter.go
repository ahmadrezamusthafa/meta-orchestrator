package shadow

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router/feedback"
)

//go:embed templates/benchmark_report.md.tmpl
var templatesFS embed.FS

var reportTemplate = template.Must(template.ParseFS(templatesFS, "templates/benchmark_report.md.tmpl"))

type reportRow struct {
	Method, Model, Tier                                      string
	Samples                                                  int
	FPVR, Tokens, Cost, CostDiff, TTR, Score, Pareto, Result string
}

type reportTuple struct {
	Stage, Complexity string
	Rows              []reportRow
}

type winnerRow struct {
	Stage string
	Cols  []string
}

type reportData struct {
	GeneratedAt, SweepID, SweepStarted, SweepFinished, SweepDuration string
	TupleCount, WinnerCount, CandidateCount, MinSamples              int
	WeightFPVR, WeightCost, WeightTTR, MinFPVR                       string
	WinnerRows                                                       []winnerRow
	Changes                                                          []string
	Tuples                                                           []reportTuple
}

// Reporter renders the markdown benchmark diff report.
type Reporter struct {
	builder *MatrixBuilder
}

// NewReporter creates a reporter using the default scoring configuration.
func NewReporter() *Reporter { return &Reporter{builder: NewMatrixBuilder(DefaultBuilderConfig())} }

// NewReporterWithBuilder renders scores with a specific builder configuration.
func NewReporterWithBuilder(b *MatrixBuilder) *Reporter { return &Reporter{builder: b} }

// ReportFileName is {timestamp}_stage_method_benchmark.md.
func ReportFileName(t time.Time) string {
	return t.UTC().Format("20060102T150405Z") + "_stage_method_benchmark.md"
}

// Render produces the markdown report. prev (optional) is the matrix being replaced.
func (r *Reporter) Render(sweep SweepResult, m BestMethodsMatrix, prev *BestMethodsMatrix) (string, error) {
	cfg := r.builder.cfg
	d := reportData{
		GeneratedAt:   m.GeneratedAt.UTC().Format(time.RFC3339),
		SweepID:       sweep.ID,
		SweepStarted:  sweep.StartedAt.UTC().Format(time.RFC3339),
		SweepFinished: sweep.FinishedAt.UTC().Format(time.RFC3339),
		SweepDuration: sweep.FinishedAt.Sub(sweep.StartedAt).Round(time.Second).String(),
		MinSamples:    cfg.MinSamples,
		WeightFPVR:    fmt.Sprintf("%.2f", cfg.Weights.FPVR),
		WeightCost:    fmt.Sprintf("%.2f", cfg.Weights.Cost),
		WeightTTR:     fmt.Sprintf("%.2f", cfg.Weights.TTR),
		MinFPVR:       fmt.Sprintf("%.0f%%", cfg.MinFPVR*100),
		WinnerCount:   len(m.Cells),
	}

	for _, st := range Stages {
		row := winnerRow{Stage: st}
		any := false
		for _, cx := range Complexities {
			if c, ok := m.Cell(st, cx); ok {
				any = true
				model := c.WinningModel
				if model == "" {
					model = "default"
				}
				row.Cols = append(row.Cols, fmt.Sprintf("**%s** · %s (%s)", c.OptimalMethod, model, c.ModelTier))
			} else {
				row.Cols = append(row.Cols, "—")
			}
		}
		if any {
			d.WinnerRows = append(d.WinnerRows, row)
		}
	}

	if prev != nil {
		for _, c := range m.Cells {
			old, ok := prev.Cell(c.StageID, c.Complexity)
			switch {
			case !ok:
				d.Changes = append(d.Changes, fmt.Sprintf("%s/%s: new winner %s on %s", c.StageID, c.Complexity, c.OptimalMethod, c.WinningModel))
			case old.OptimalMethod != c.OptimalMethod || old.WinningModel != c.WinningModel:
				d.Changes = append(d.Changes, fmt.Sprintf("%s/%s: %s on %s → %s on %s (cost $%.4f → $%.4f, FPVR %.1f%% → %.1f%%)",
					c.StageID, c.Complexity, old.OptimalMethod, orDefault(old.WinningModel), c.OptimalMethod, c.WinningModel,
					old.AvgCostUSD, c.AvgCostUSD, old.FPVRPercent, c.FPVRPercent))
			}
		}
	}

	for _, t := range r.builder.Score(sweep.Cells) {
		d.TupleCount++
		rt := reportTuple{Stage: t.Stage, Complexity: t.Complexity}
		var winnerCost float64
		hasWinner := len(t.Candidates) > 0 && t.Candidates[0].Eligible
		if hasWinner {
			winnerCost = t.Candidates[0].Result.AvgCostUSD
		}
		for i, c := range t.Candidates {
			d.CandidateCount++
			res := c.Result
			lo, hi := feedback.WilsonInterval(res.FirstPasses, res.Samples)
			row := reportRow{Method: res.Cell.Method, Model: res.Cell.Model, Tier: res.Cell.Tier, Samples: res.Samples,
				FPVR:   fmt.Sprintf("%.1f%% [%.1f–%.1f]", res.FPVR*100, lo*100, hi*100),
				Tokens: fmt.Sprintf("%.0f", res.AvgTokens), Cost: fmt.Sprintf("$%.4f", res.AvgCostUSD),
				TTR: fmt.Sprintf("%.1fs", res.AvgTTRSeconds), CostDiff: "—", Score: "—", Pareto: "", Result: ""}
			if c.Eligible {
				row.Score = fmt.Sprintf("%.3f", c.Score)
			} else {
				row.Result = c.Ineligible
			}
			if c.Pareto {
				row.Pareto = "✓"
			}
			switch {
			case i == 0 && hasWinner:
				row.Result = "**winner**"
				row.CostDiff = "baseline"
			case hasWinner && winnerCost > 0:
				row.CostDiff = fmt.Sprintf("%+.1f%%", (res.AvgCostUSD-winnerCost)/winnerCost*100)
			case hasWinner:
				row.CostDiff = fmt.Sprintf("%+.4f USD", res.AvgCostUSD-winnerCost)
			}
			if res.Errors > 0 {
				row.Result = fmt.Sprintf("%s %d errors", row.Result, res.Errors)
			}
			rt.Rows = append(rt.Rows, row)
		}
		d.Tuples = append(d.Tuples, rt)
	}

	var buf bytes.Buffer
	if err := reportTemplate.Execute(&buf, d); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func orDefault(s string) string {
	if s == "" {
		return "default"
	}
	return s
}

// Write renders the report into dir/{timestamp}_stage_method_benchmark.md and returns its path.
func (r *Reporter) Write(dir string, sweep SweepResult, m BestMethodsMatrix, prev *BestMethodsMatrix) (string, error) {
	md, err := r.Render(sweep, m, prev)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, ReportFileName(m.GeneratedAt))
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// Publisher compiles a sweep into the active matrix: build → merge with current → report → hot-swap.
type Publisher struct {
	Builder   *MatrixBuilder
	Reporter  *Reporter
	Store     *MatrixStore
	ReportDir string // e.g. .sdlc/benchmarks
	Now       func() time.Time
}

// Publish finalizes a sweep and returns the new matrix and the report path.
func (p *Publisher) Publish(ctx context.Context, sweep SweepResult) (BestMethodsMatrix, string, error) {
	if err := ctx.Err(); err != nil {
		return BestMethodsMatrix{}, "", err
	}
	now := time.Now
	if p.Now != nil {
		now = p.Now
	}
	prev := p.Store.Current()
	next := p.Builder.Build(sweep.ID, sweep.Cells, now())
	// Benchmark cells replace defaults; a default-source matrix contributes nothing to merge.
	merged := next
	if prev.Source == SourceShadowBenchmark {
		merged = MergeMatrix(prev, next)
	}
	path, err := p.Reporter.Write(p.ReportDir, sweep, merged, &prev)
	if err != nil {
		return BestMethodsMatrix{}, "", err
	}
	if err := p.Store.Update(merged, path); err != nil {
		return BestMethodsMatrix{}, "", err
	}
	return merged, path, nil
}
