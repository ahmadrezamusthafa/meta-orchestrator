package feedback

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/telemetry"
)

// CalibratorConfig tunes how heuristic scores become routing weights.
type CalibratorConfig struct {
	Heuristics Config `json:"heuristics"`
	// ApplyThreshold is the minimum calibrated success probability of the preferred tier
	// before best-practice routing adopts it.
	ApplyThreshold float64 `json:"apply_threshold"`
	// PromotionMinRuns is the run count a custom rule needs before it can be promoted.
	PromotionMinRuns int `json:"promotion_min_runs"`
	// ReservedComplexities always keep Tier 1 unless an admin lock says otherwise.
	ReservedComplexities []string `json:"reserved_complexities"`
	// ReservedTaskTypes always keep Tier 1 (novel / architectural work).
	ReservedTaskTypes []string `json:"reserved_task_types"`
}

// DefaultCalibratorConfig returns the plan's defaults (20+ runs for promotion).
func DefaultCalibratorConfig() CalibratorConfig {
	return CalibratorConfig{
		Heuristics:           DefaultConfig(),
		ApplyThreshold:       0.85,
		PromotionMinRuns:     20,
		ReservedComplexities: []string{"HIGH", "CRITICAL", "SYSTEM"},
		ReservedTaskTypes:    []string{"architecture"},
	}
}

// PromotionRecommendation suggests promoting a project custom rule to a system-wide best practice.
type PromotionRecommendation struct {
	RuleID                 string  `json:"rule_id"`
	TaskType               string  `json:"task_type"`
	Runs                   int     `json:"runs"`
	CustomFPVR             float64 `json:"custom_fpvr"`
	BestPracticeFPVR       float64 `json:"best_practice_fpvr"`
	CustomAvgCostUSD       float64 `json:"custom_avg_cost_usd"`
	BestPracticeAvgCostUSD float64 `json:"best_practice_avg_cost_usd"`
	Message                string  `json:"message"`
}

// CalibrationResult reports what a calibration pass changed.
type CalibrationResult struct {
	Updated    []Weight                  `json:"updated"`
	Skipped    []string                  `json:"skipped_locked"`
	Promotions []PromotionRecommendation `json:"promotions"`
	Report     Report                    `json:"report"`
}

// Calibrator feeds heuristic scores back into routing weights. It implements router.TierAdvisor.
type Calibrator struct {
	mu        sync.RWMutex
	store     WeightStore
	cfg       CalibratorConfig
	evaluator *Evaluator
	cache     map[string]Weight
	now       func() time.Time
}

// NewCalibrator creates a calibrator backed by store.
func NewCalibrator(store WeightStore, cfg CalibratorConfig) *Calibrator {
	if store == nil {
		store = NewMemoryWeightStore()
	}
	return &Calibrator{store: store, cfg: cfg, evaluator: NewEvaluator(cfg.Heuristics), cache: map[string]Weight{}, now: time.Now}
}

// Load refreshes the in-memory weight cache from the store.
func (c *Calibrator) Load(ctx context.Context) error {
	all, err := c.store.List(ctx)
	if err != nil {
		return err
	}
	cache := make(map[string]Weight, len(all))
	for _, w := range all {
		cache[w.TaskType] = w
	}
	c.mu.Lock()
	c.cache = cache
	c.mu.Unlock()
	return nil
}

// Weights returns the current weight table.
func (c *Calibrator) Weights(ctx context.Context) ([]Weight, error) { return c.store.List(ctx) }

// Calibrate evaluates history, rewrites unlocked weights and returns promotion suggestions.
func (c *Calibrator) Calibrate(ctx context.Context, runs []telemetry.RunRecord) (CalibrationResult, error) {
	rep := c.evaluator.Evaluate(runs)
	res := CalibrationResult{Updated: []Weight{}, Skipped: []string{}, Promotions: []PromotionRecommendation{}, Report: rep}

	tierWeights := map[string]map[string]float64{}
	tierRuns := map[string]int{}
	byTypeTier := map[string]map[string]*accumulator{}
	for _, r := range runs {
		tier := NormalizeTier(r.Tier)
		if tierRank(tier) == 0 {
			continue
		}
		if byTypeTier[r.Category] == nil {
			byTypeTier[r.Category] = map[string]*accumulator{}
		}
		if byTypeTier[r.Category][tier] == nil {
			byTypeTier[r.Category][tier] = &accumulator{}
		}
		byTypeTier[r.Category][tier].add(r)
		tierRuns[r.Category]++
	}
	for taskType, tiers := range byTypeTier {
		tierWeights[taskType] = map[string]float64{}
		for tier, a := range tiers {
			tierWeights[taskType][tier] = a.score(CellKey{}).SuccessProbability
		}
	}

	downgrade := map[string]DowngradeCandidate{}
	for _, d := range rep.Downgrades {
		downgrade[d.TaskType] = d
	}

	now := c.now()
	for taskType, weights := range tierWeights {
		existing, found, err := c.store.Get(ctx, taskType)
		if err != nil {
			return res, err
		}
		if found && existing.Locked {
			res.Skipped = append(res.Skipped, taskType)
			continue
		}
		w := Weight{TaskType: taskType, TierWeights: weights, Runs: tierRuns[taskType], UpdatedAt: now}
		if d, ok := downgrade[taskType]; ok {
			w.PreferredTier = d.ToTier
			w.Reason = fmt.Sprintf("%s passes %.0f%% first-pass over %d runs on %s (est. %.0f%% cheaper than %s)",
				taskType, d.PassRate*100, d.Runs, d.ToTier, d.EstimatedSavingsPct, d.FromTier)
		}
		if err := c.store.Set(ctx, w); err != nil {
			return res, err
		}
		res.Updated = append(res.Updated, w)
	}

	for _, cmp := range rep.RuleComparisons {
		if cmp.Winner != WinnerCustom || cmp.CustomRuns < c.cfg.PromotionMinRuns {
			continue
		}
		if cmp.CustomFPVR <= cmp.BestPracticeFPVR || cmp.CustomAvgCostUSD >= cmp.BestPracticeAvgCostUSD {
			continue
		}
		res.Promotions = append(res.Promotions, PromotionRecommendation{
			RuleID: cmp.RuleID, TaskType: cmp.TaskType, Runs: cmp.CustomRuns,
			CustomFPVR: cmp.CustomFPVR, BestPracticeFPVR: cmp.BestPracticeFPVR,
			CustomAvgCostUSD: cmp.CustomAvgCostUSD, BestPracticeAvgCostUSD: cmp.BestPracticeAvgCostUSD,
			Message: fmt.Sprintf("Promote Custom Rule %s to Best Practice for %s: FPVR %.0f%% vs %.0f%%, avg cost $%.4f vs $%.4f over %d runs",
				cmp.RuleID, cmp.TaskType, cmp.CustomFPVR*100, cmp.BestPracticeFPVR*100, cmp.CustomAvgCostUSD, cmp.BestPracticeAvgCostUSD, cmp.CustomRuns),
		})
	}

	return res, c.Load(ctx)
}

var validTiers = map[string]bool{"tier1": true, "tier2": true, "tier3": true}

// Lock pins a task type to a tier (admin override). Calibration leaves locked weights untouched.
func (c *Calibrator) Lock(ctx context.Context, taskType, tier, by, reason string) error {
	tier = NormalizeTier(tier)
	if !validTiers[tier] {
		return fmt.Errorf("invalid tier %q (want tier1, tier2 or tier3)", tier)
	}
	if strings.TrimSpace(taskType) == "" {
		return fmt.Errorf("task type is required")
	}
	w, _, err := c.store.Get(ctx, taskType)
	if err != nil {
		return err
	}
	w.TaskType = taskType
	w.PreferredTier = tier
	w.Locked = true
	w.LockedBy = by
	w.Reason = fmt.Sprintf("Locked by %s: %s", by, reason)
	w.UpdatedAt = c.now()
	if err := c.store.Set(ctx, w); err != nil {
		return err
	}
	return c.Load(ctx)
}

// Unlock releases an admin lock; the next calibration may change the weight again.
func (c *Calibrator) Unlock(ctx context.Context, taskType string) error {
	w, ok, err := c.store.Get(ctx, taskType)
	if err != nil || !ok {
		return err
	}
	w.Locked = false
	w.LockedBy = ""
	w.PreferredTier = ""
	w.Reason = "unlocked; awaiting recalibration"
	w.UpdatedAt = c.now()
	if err := c.store.Set(ctx, w); err != nil {
		return err
	}
	return c.Load(ctx)
}

// AdviseTier implements router.TierAdvisor. Locked weights always apply. Calibrated weights only
// move routing to a cheaper tier, only above ApplyThreshold, and never for reserved work.
func (c *Calibrator) AdviseTier(taskType, complexity, stageID, currentTier string) (string, string, bool) {
	c.mu.RLock()
	w, ok := c.cache[taskType]
	c.mu.RUnlock()
	if !ok || w.PreferredTier == "" {
		return "", "", false
	}
	if w.Locked {
		return w.PreferredTier, w.Reason, true
	}
	for _, rc := range c.cfg.ReservedComplexities {
		if strings.EqualFold(rc, complexity) {
			return "", "", false
		}
	}
	for _, rt := range c.cfg.ReservedTaskTypes {
		if strings.EqualFold(rt, taskType) {
			return "", "", false
		}
	}
	if tierRank(w.PreferredTier) <= tierRank(NormalizeTier(currentTier)) {
		return "", "", false
	}
	if w.TierWeights[w.PreferredTier] < c.cfg.ApplyThreshold {
		return "", "", false
	}
	return w.PreferredTier, w.Reason, true
}
