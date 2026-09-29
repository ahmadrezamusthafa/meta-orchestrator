package telemetry

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// ModelPrice is the USD list price per one million tokens for a model.
type ModelPrice struct {
	InputPer1M       float64 `json:"input_per_1m"`
	OutputPer1M      float64 `json:"output_per_1m"`
	CachedInputPer1M float64 `json:"cached_input_per_1m"`
}

// defaultPrices are public list prices at the time of writing. They are defaults only:
// override them via LoadFromFile (configs/pricing.json) to match the account's actual billing.
var defaultPrices = map[string]ModelPrice{
	"claude-3-5-sonnet": {InputPer1M: 3.00, OutputPer1M: 15.00, CachedInputPer1M: 0.30},
	"claude-3-7-sonnet": {InputPer1M: 3.00, OutputPer1M: 15.00, CachedInputPer1M: 0.30},
	"claude-3-5-haiku":  {InputPer1M: 0.80, OutputPer1M: 4.00, CachedInputPer1M: 0.08},
	"claude-3-opus":     {InputPer1M: 15.00, OutputPer1M: 75.00, CachedInputPer1M: 1.50},
	"gpt-4o":            {InputPer1M: 2.50, OutputPer1M: 10.00, CachedInputPer1M: 1.25},
	"gpt-4o-mini":       {InputPer1M: 0.15, OutputPer1M: 0.60, CachedInputPer1M: 0.075},
	"gemini-2.0-flash":  {InputPer1M: 0.10, OutputPer1M: 0.40, CachedInputPer1M: 0.025},
	"gemini-1.5-pro":    {InputPer1M: 1.25, OutputPer1M: 5.00, CachedInputPer1M: 0.3125},
	"deepseek-coder":    {InputPer1M: 0, OutputPer1M: 0, CachedInputPer1M: 0}, // local / self-hosted
	"opencode":          {InputPer1M: 0, OutputPer1M: 0, CachedInputPer1M: 0},
}

var dateSuffix = regexp.MustCompile(`-(\d{8}|latest)$`)

// PricingTable resolves per-model token prices and computes call cost.
type PricingTable struct {
	mu     sync.RWMutex
	prices map[string]ModelPrice
	keys   []string // sorted longest-first for prefix matching
}

// NewPricingTable returns a table seeded with default list prices.
func NewPricingTable() *PricingTable {
	p := &PricingTable{prices: make(map[string]ModelPrice)}
	for k, v := range defaultPrices {
		p.prices[k] = v
	}
	p.reindex()
	return p
}

// Set overrides or adds the price for a model key.
func (p *PricingTable) Set(model string, price ModelPrice) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.prices[normalizeModel(model)] = price
	p.reindex()
}

// LoadFromFile merges a JSON object of {model: ModelPrice} over the current table.
func (p *PricingTable) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var overrides map[string]ModelPrice
	if err := json.Unmarshal(data, &overrides); err != nil {
		return fmt.Errorf("invalid pricing file %s: %w", path, err)
	}
	for k, v := range overrides {
		p.Set(k, v)
	}
	return nil
}

// Lookup resolves a model identifier (optionally provider-prefixed and date-suffixed)
// to its price using exact then longest-prefix matching.
func (p *PricingTable) Lookup(model string) (ModelPrice, bool) {
	name := normalizeModel(model)
	p.mu.RLock()
	defer p.mu.RUnlock()
	if price, ok := p.prices[name]; ok {
		return price, true
	}
	for _, k := range p.keys {
		if strings.HasPrefix(name, k) {
			rest := name[len(k):]
			// Only accept a boundary match so "gpt-4o" does not swallow "gpt-4o-mini"
			// when the latter is priced; the longest-first ordering handles that case.
			if rest == "" || strings.HasPrefix(rest, "-") || strings.HasPrefix(rest, ":") {
				return p.prices[k], true
			}
		}
	}
	return ModelPrice{}, false
}

// Cost computes the USD cost of a call. promptTokens includes cachedTokens;
// cached tokens are billed at the cached-input rate, the remainder at the input rate.
// Unknown models cost 0.
func (p *PricingTable) Cost(model string, promptTokens, completionTokens, cachedTokens int64) float64 {
	price, ok := p.Lookup(model)
	if !ok {
		return 0
	}
	if cachedTokens > promptTokens {
		cachedTokens = promptTokens
	}
	if cachedTokens < 0 {
		cachedTokens = 0
	}
	uncached := promptTokens - cachedTokens
	return float64(uncached)*price.InputPer1M/1e6 +
		float64(cachedTokens)*price.CachedInputPer1M/1e6 +
		float64(completionTokens)*price.OutputPer1M/1e6
}

func (p *PricingTable) reindex() {
	keys := make([]string, 0, len(p.prices))
	for k := range p.prices {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	p.keys = keys
}

func normalizeModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	if idx := strings.LastIndex(m, "/"); idx >= 0 {
		m = m[idx+1:]
	}
	return dateSuffix.ReplaceAllString(m, "")
}
