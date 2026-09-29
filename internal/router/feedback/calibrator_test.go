package feedback

import (
	"context"
	"os"
	"testing"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/config"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
	"github.com/redis/go-redis/v9"
)

func tieredConfig() *config.OrchestratorConfig {
	cfg := config.GetDefaultConfig()
	cfg.ModelTiers.Tier1Reasoning = "claude/claude-3-5-sonnet-20241022"
	cfg.ModelTiers.Tier2CodeGen = "openai/gpt-4o-mini"
	cfg.ModelTiers.Tier3LogParse = "claude/claude-3-5-haiku-20241022"
	cfg.Router.Strategy = "best_practice"
	return cfg
}

func TestClassifyTaskType(t *testing.T) {
	cases := map[string]string{
		"Add CRUD endpoints for invoice line items":  "crud",
		"Create REST API to update customer profile": "crud",
		"Fix typo in README":                         "docs",
		"Database migration for billing schema":      "migration",
		"Design new event-sourcing architecture":     "architecture",
		"Fix null pointer bug in webhook handler":    "bugfix",
		"Something entirely different":               "general",
	}
	for title, want := range cases {
		if got := router.ClassifyTaskType(title, ""); got != want {
			t.Errorf("ClassifyTaskType(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestCalibratorShiftsRoutineCRUDFromTier1ToTier2(t *testing.T) {
	ctx := context.Background()
	r := router.NewRouter(tieredConfig())

	before := r.RouteForTask("INTAKE_PRD", "MEDIUM", "crud", nil)
	if before.Tier != "tier1" || before.Model != "claude/claude-3-5-sonnet-20241022" {
		t.Fatalf("baseline decision = %+v", before)
	}

	cal := NewCalibrator(NewMemoryWeightStore(), DefaultCalibratorConfig())
	r.SetTierAdvisor(cal)

	// Below threshold: too few runs, nothing shifts.
	few := build(
		runSpec{category: "crud", method: "ReAct", tier: "tier1", strategy: "best_practice", n: 3, firstPasses: 3, cost: 0.2},
		runSpec{category: "crud", method: "ReAct", tier: "tier2", strategy: "best_practice", n: 3, firstPasses: 3, cost: 0.03},
	)
	if _, err := cal.Calibrate(ctx, few); err != nil {
		t.Fatal(err)
	}
	if d := r.RouteForTask("INTAKE_PRD", "MEDIUM", "crud", nil); d.Tier != "tier1" {
		t.Fatalf("shifted before threshold met: %+v", d)
	}

	res, err := cal.Calibrate(ctx, historicalDataset())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Updated) == 0 {
		t.Fatal("expected weight updates")
	}

	after := r.RouteForTask("INTAKE_PRD", "MEDIUM", "crud", nil)
	if after.Tier != "tier2" || after.Model != "openai/gpt-4o-mini" {
		t.Fatalf("CRUD not shifted to tier2: %+v", after)
	}
	if after.Strategy != "best_practice" || after.Reasoning == before.Reasoning {
		t.Fatalf("decision should explain the calibrated shift: %+v", after)
	}
	// Identical subsequent requests stay shifted.
	if again := r.RouteForTask("INTAKE_PRD", "MEDIUM", "crud", nil); again.Tier != "tier2" {
		t.Fatalf("subsequent identical request = %+v", again)
	}
	// Tier 1 stays reserved for novel/architectural work.
	if hi := r.RouteForTask("INTAKE_PRD", "HIGH", "crud", nil); hi.Tier != "tier1" {
		t.Fatalf("HIGH complexity must keep tier1: %+v", hi)
	}
	if arch := r.RouteForTask("INTAKE_PRD", "MEDIUM", "architecture", nil); arch.Tier != "tier1" {
		t.Fatalf("architecture must keep tier1: %+v", arch)
	}
	// Migration stays on tier1 (tier2 failed the bar).
	if mig := r.RouteForTask("INTAKE_PRD", "MEDIUM", "migration", nil); mig.Tier != "tier1" {
		t.Fatalf("migration shifted: %+v", mig)
	}
	// Route() without a task type is unaffected.
	if plain := r.Route("INTAKE_PRD", "MEDIUM", nil); plain.Tier != "tier1" {
		t.Fatalf("Route without task type = %+v", plain)
	}
}

func TestCalibratorRecommendsCustomRulePromotion(t *testing.T) {
	cal := NewCalibrator(NewMemoryWeightStore(), DefaultCalibratorConfig())
	res, err := cal.Calibrate(context.Background(), historicalDataset())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Promotions) != 1 {
		t.Fatalf("promotions = %+v", res.Promotions)
	}
	p := res.Promotions[0]
	if p.RuleID != "rule-crud-bmad" || p.TaskType != "crud" || p.Runs < 20 || p.CustomFPVR <= p.BestPracticeFPVR || p.CustomAvgCostUSD >= p.BestPracticeAvgCostUSD {
		t.Fatalf("promotion = %+v", p)
	}
	if p.Message == "" {
		t.Fatal("promotion needs an operator-facing message")
	}

	// The same rule with only 19 runs is not promoted.
	short := build(
		runSpec{category: "crud", method: "ReAct", tier: "tier1", strategy: "best_practice", n: 20, firstPasses: 15, cost: 0.2},
		runSpec{category: "crud", method: "BMAD", tier: "tier2", strategy: "custom", rule: "r19", n: 19, firstPasses: 19, cost: 0.02},
	)
	res, _ = NewCalibrator(NewMemoryWeightStore(), DefaultCalibratorConfig()).Calibrate(context.Background(), short)
	if len(res.Promotions) != 0 {
		t.Fatalf("promoted with 19 runs: %+v", res.Promotions)
	}
}

func TestAdminLockOverridesCalibration(t *testing.T) {
	ctx := context.Background()
	r := router.NewRouter(tieredConfig())
	cal := NewCalibrator(NewMemoryWeightStore(), DefaultCalibratorConfig())
	r.SetTierAdvisor(cal)

	if err := cal.Lock(ctx, "crud", "tier1", "admin", "compliance review"); err != nil {
		t.Fatal(err)
	}
	if _, err := cal.Calibrate(ctx, historicalDataset()); err != nil {
		t.Fatal(err)
	}
	d := r.RouteForTask("INTAKE_PRD", "MEDIUM", "crud", nil)
	if d.Tier != "tier1" {
		t.Fatalf("locked rule overridden by calibration: %+v", d)
	}
	w, ok, _ := cal.store.Get(ctx, "crud")
	if !ok || !w.Locked || w.LockedBy != "admin" || w.PreferredTier != "tier1" {
		t.Fatalf("weight = %+v", w)
	}

	// A lock applies even to HIGH complexity (explicit admin intent).
	if err := cal.Lock(ctx, "migration", "tier2", "admin", "cost freeze"); err != nil {
		t.Fatal(err)
	}
	if hi := r.RouteForTask("INTAKE_PRD", "HIGH", "migration", nil); hi.Tier != "tier2" {
		t.Fatalf("admin lock ignored: %+v", hi)
	}

	if err := cal.Unlock(ctx, "crud"); err != nil {
		t.Fatal(err)
	}
	if _, err := cal.Calibrate(ctx, historicalDataset()); err != nil {
		t.Fatal(err)
	}
	if d := r.RouteForTask("INTAKE_PRD", "MEDIUM", "crud", nil); d.Tier != "tier2" {
		t.Fatalf("after unlock calibration should apply: %+v", d)
	}
	if err := cal.Lock(ctx, "crud", "tier7", "admin", ""); err == nil {
		t.Fatal("invalid tier should be rejected")
	}
}

func TestCalibratorReloadsWeightsFromStore(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryWeightStore()
	if _, err := NewCalibrator(store, DefaultCalibratorConfig()).Calibrate(ctx, historicalDataset()); err != nil {
		t.Fatal(err)
	}
	fresh := NewCalibrator(store, DefaultCalibratorConfig())
	if err := fresh.Load(ctx); err != nil {
		t.Fatal(err)
	}
	if tier, _, ok := fresh.AdviseTier("crud", "MEDIUM", "INTAKE_PRD", "tier1"); !ok || tier != "tier2" {
		t.Fatalf("reloaded advice = %s %v", tier, ok)
	}
}

// TestRedisWeightStore runs against a real Redis only when REDIS_ADDR is set.
func TestRedisWeightStore(t *testing.T) {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		t.Skip("REDIS_ADDR not set")
	}
	ctx := context.Background()
	client := redis.NewClient(&redis.Options{Addr: addr})
	store := NewRedisWeightStore(client, "test:router:weights")
	defer client.Del(ctx, "test:router:weights")
	if err := store.Set(ctx, Weight{TaskType: "crud", PreferredTier: "tier2", TierWeights: map[string]float64{"tier2": 0.9}}); err != nil {
		t.Fatal(err)
	}
	w, ok, err := store.Get(ctx, "crud")
	if err != nil || !ok || w.PreferredTier != "tier2" {
		t.Fatalf("get = %+v %v %v", w, ok, err)
	}
	all, err := store.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list = %+v %v", all, err)
	}
}
