package telemetry

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var refNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func synthRun(i int, at time.Time) RunRecord {
	methods := []string{"BMAD", "Supervisor", "ReAct", "Superpower"}
	models := []string{"claude/claude-3-5-sonnet", "openai/gpt-4o-mini"}
	repos := []string{"backend-core", "frontend-portal"}
	cats := []string{"crud", "migration"}
	return RunRecord{
		RunMeta: RunMeta{
			TaskID: fmt.Sprintf("T-%d", i), Model: models[i%2], Tier: []string{"tier1", "tier2"}[i%2],
			Method: methods[i%4], Repo: repos[i%2], Category: cats[i%2], Complexity: "MEDIUM",
			RouterStrategy: "best_practice",
		},
		Success:        i%10 != 0,
		FirstPass:      i%3 != 0,
		TestIterations: 1 + i%3,
		FailureLoops:   i % 3,
		DurationUS:     int64(60+i%120) * 1_000_000,
		PromptTokens:   1000, CompletionTokens: 500, CostUSD: 0.01,
		StartedAt: at, FinishedAt: at,
	}
}

func TestDatastorePersistsAndReloadsTables(t *testing.T) {
	dir := t.TempDir()
	ds, err := OpenDatastore(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = ds.RecordTokens(TokenEvent{CallMeta: CallMeta{TaskID: "A", Repo: "r1", Method: "BMAD"}, PromptTokens: 10, CompletionTokens: 5, CostUSD: 0.1, Timestamp: refNow})
	_ = ds.RecordRun(synthRun(1, refNow))
	_ = ds.RecordRun(synthRun(2, refNow))
	if err := ds.Close(); err != nil {
		t.Fatal(err)
	}

	re, err := OpenDatastore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer re.Close()
	if n := len(re.Tokens(Filter{})); n != 1 {
		t.Fatalf("reloaded %d token rows", n)
	}
	if n := len(re.Runs(Filter{})); n != 2 {
		t.Fatalf("reloaded %d run rows", n)
	}
	if n := len(re.MethodStats()); n != 2 {
		t.Fatalf("telemetry_methods rows = %d, want 2 (Supervisor, ReAct)", n)
	}
}

func TestDatastoreFiltersWindowRepoAndShadow(t *testing.T) {
	ds := NewMemoryDatastore()
	old := synthRun(1, refNow.Add(-10*24*time.Hour))
	recent := synthRun(2, refNow.Add(-time.Hour))
	shadow := synthRun(3, refNow.Add(-time.Hour))
	shadow.Shadow = true
	for _, r := range []RunRecord{old, recent, shadow} {
		_ = ds.RecordRun(r)
	}
	if n := len(ds.Runs(Filter{Since: refNow.Add(-7 * 24 * time.Hour)})); n != 1 {
		t.Fatalf("7d window runs = %d, want 1 (shadow excluded)", n)
	}
	if n := len(ds.Runs(Filter{IncludeShadow: true})); n != 3 {
		t.Fatalf("all runs incl shadow = %d", n)
	}
	if n := len(ds.Runs(Filter{OnlyShadow: true})); n != 1 {
		t.Fatalf("only shadow = %d", n)
	}
	// synthRun alternates repos: odd indexes (1 and 3) are frontend-portal.
	if n := len(ds.Runs(Filter{Repo: "frontend-portal", IncludeShadow: true})); n != 2 {
		t.Fatalf("repo filter = %d, want 2", n)
	}
}

func TestSummaryAggregations(t *testing.T) {
	ds := NewMemoryDatastore()
	// Two crud runs on tier2, one success first-pass (60s), one success not first-pass (120s).
	_ = ds.RecordRun(RunRecord{RunMeta: RunMeta{TaskID: "a", Model: "m2", Tier: "tier2", Method: "ReAct", Repo: "r", Category: "crud"},
		Success: true, FirstPass: true, TestIterations: 1, DurationUS: 60e6, PromptTokens: 800, CompletionTokens: 200, CostUSD: 0.02, FinishedAt: refNow.Add(-time.Hour)})
	_ = ds.RecordRun(RunRecord{RunMeta: RunMeta{TaskID: "b", Model: "m2", Tier: "tier2", Method: "ReAct", Repo: "r", Category: "crud"},
		Success: true, FirstPass: false, TestIterations: 3, FailureLoops: 2, DurationUS: 120e6, PromptTokens: 1600, CompletionTokens: 400, CostUSD: 0.04, FinishedAt: refNow.Add(-time.Hour)})
	// One failed migration run on tier1.
	_ = ds.RecordRun(RunRecord{RunMeta: RunMeta{TaskID: "c", Model: "m1", Tier: "tier1", Method: "BMAD", Repo: "r", Category: "migration"},
		Success: false, TestIterations: 2, FailureLoops: 2, DurationUS: 300e6, PromptTokens: 5000, CompletionTokens: 1000, CostUSD: 0.5, FinishedAt: refNow.Add(-time.Hour)})
	_ = ds.RecordTokens(TokenEvent{CallMeta: CallMeta{TaskID: "a", Repo: "r"}, PromptTokens: 800, CompletionTokens: 200, CachedTokens: 100, CostUSD: 0.02, Timestamp: refNow.Add(-time.Hour)})

	s := Summarize(ds, "7d", "", refNow)
	if s.Totals.Runs != 3 {
		t.Fatalf("runs = %d", s.Totals.Runs)
	}
	if math.Abs(s.Totals.FPVRPercent-100.0/3) > 1e-9 {
		t.Fatalf("fpvr = %v", s.Totals.FPVRPercent)
	}
	if s.Totals.MTTRSeconds != 90 {
		t.Fatalf("mttr = %v, want 90 (mean of successful runs)", s.Totals.MTTRSeconds)
	}
	if s.Totals.CachedTokens != 100 || s.Totals.PromptTokens != 800 {
		t.Fatalf("token totals should come from telemetry_tokens: %+v", s.Totals)
	}
	var crud CategoryStat
	for _, c := range s.ByCategory {
		if c.Category == "crud" {
			crud = c
		}
	}
	if crud.Runs != 2 || crud.AvgTPFTokens != 1500 || math.Abs(crud.AvgCostUSD-0.03) > 1e-12 || crud.FPVRPercent != 50 {
		t.Fatalf("crud = %+v", crud)
	}
	if len(s.Leaderboard) != 2 || s.Leaderboard[0].Model != "m2" || s.Leaderboard[0].TasksCompleted != 2 {
		t.Fatalf("leaderboard = %+v", s.Leaderboard)
	}
	if len(s.Stability) != 1 || s.Stability[0].FailureLoops != 4 || math.Abs(s.Stability[0].FlakinessIndex-4.0/6) > 1e-12 {
		t.Fatalf("stability = %+v", s.Stability)
	}
}

func TestTrendsFillsDailyBuckets(t *testing.T) {
	ds := NewMemoryDatastore()
	_ = ds.RecordTokens(TokenEvent{PromptTokens: 100, CompletionTokens: 50, CostUSD: 1, Timestamp: refNow.Add(-2 * 24 * time.Hour)})
	_ = ds.RecordTokens(TokenEvent{PromptTokens: 100, CompletionTokens: 50, CostUSD: 1, Timestamp: refNow.Add(-2*24*time.Hour + time.Hour)})
	tr := ComputeTrends(ds, "7d", "", refNow)
	if tr.Bucket != "day" || len(tr.Burn) != 7 || len(tr.MTTR) != 7 {
		t.Fatalf("buckets = %s %d %d", tr.Bucket, len(tr.Burn), len(tr.MTTR))
	}
	var hit BurnPoint
	for _, b := range tr.Burn {
		if b.Date == "2026-09-26" {
			hit = b
		}
	}
	if hit.PromptTokens != 200 || hit.CompletionTokens != 100 || hit.CostUSD != 2 {
		t.Fatalf("2026-09-26 bucket = %+v (burn=%+v)", hit, tr.Burn)
	}
	if h := ComputeTrends(ds, "24h", "", refNow); h.Bucket != "hour" || len(h.Burn) != 24 {
		t.Fatalf("24h trends = %s/%d", h.Bucket, len(h.Burn))
	}
}

func TestTelemetryAPIWith1000RowsUnder25ms(t *testing.T) {
	ds := NewMemoryDatastore()
	for i := 0; i < 1000; i++ {
		at := refNow.Add(-time.Duration(i) * 10 * time.Minute)
		_ = ds.RecordRun(synthRun(i, at))
		_ = ds.RecordTokens(TokenEvent{CallMeta: CallMeta{TaskID: fmt.Sprintf("T-%d", i), Repo: "backend-core"},
			PromptTokens: 1000, CompletionTokens: 500, CostUSD: 0.01, Timestamp: at})
	}
	h := NewHandler(ds)
	h.now = func() time.Time { return refNow }

	for _, path := range []string{"/api/v1/telemetry/summary?window=7d", "/api/v1/telemetry/trends?window=7d", "/api/v1/telemetry/summary?window=all&repo=backend-core"} {
		rec := httptest.NewRecorder()
		start := time.Now()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		elapsed := time.Since(start)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s → %d: %s", path, rec.Code, rec.Body.String())
		}
		if elapsed >= 25*time.Millisecond {
			t.Fatalf("%s took %v, want < 25ms", path, elapsed)
		}
		var body map[string]interface{}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body["window"] == nil {
			t.Fatalf("%s missing window: %v", path, body)
		}
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/summary?window=7d", nil))
	var s Summary
	_ = json.Unmarshal(rec.Body.Bytes(), &s)
	if s.Totals.Runs != 1000 || len(s.Methods) != 4 || len(s.Repos) != 2 {
		t.Fatalf("summary runs=%d methods=%d repos=%v", s.Totals.Runs, len(s.Methods), s.Repos)
	}

	bad := httptest.NewRecorder()
	h.ServeHTTP(bad, httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/summary?window=13d", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid window → %d, want 400", bad.Code)
	}
	post := httptest.NewRecorder()
	h.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/summary", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST → %d, want 405", post.Code)
	}
}
