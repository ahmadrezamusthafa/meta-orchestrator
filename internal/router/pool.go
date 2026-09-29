package router

import (
	"fmt"
	"strings"
)

// Tier identifiers shared by best-practice routing, the advisors and chain item tags.
const (
	TierReasoning = "tier1"
	TierCodeGen   = "tier2"
	TierLogParse  = "tier3"
)

// Suggestion actions: what the operator must do before the router may use a recommended model.
const (
	SuggestAdd     = "add"     // not in the chain
	SuggestEnable  = "enable"  // in the chain but disabled
	SuggestConnect = "connect" // enabled, but its provider is not usable
)

// ModelSuggestion is a recommended model the router did NOT use because the operator's chain does
// not allow it. The chain is the allow-list: recommendations outside it are surfaced, never run.
type ModelSuggestion struct {
	Model  string `json:"model"`
	Tier   string `json:"tier,omitempty"`
	Action string `json:"action"`
	Reason string `json:"reason"`
}

// ProviderAvailability reports whether a provider can serve requests right now. It must be cheap
// (no network) and return true when the state is unknown, so failover still gets to try.
type ProviderAvailability func(provider string) bool

// FullModel is the routable "provider/model" string for a chain item.
func (p PriorityModelItem) FullModel() string {
	if strings.Contains(p.Model, "/") {
		return p.Model
	}
	return p.Provider + "/" + p.Model
}

// EffectiveTiers are the item's explicit tier tags, or tiers inferred from the model family.
func (p PriorityModelItem) EffectiveTiers() []string {
	if len(p.Tiers) > 0 {
		return p.Tiers
	}
	return InferTiers(p.Model)
}

func (p PriorityModelItem) hasTier(tier string) bool {
	for _, t := range p.EffectiveTiers() {
		if t == tier {
			return true
		}
	}
	return false
}

// InferTiers guesses which tiers a model suits from its family name. Operators override it by
// tagging the chain item explicitly.
func InferTiers(model string) []string {
	m := strings.ToLower(model)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(m, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("haiku", "-mini", "-7b", "-8b"):
		return []string{TierLogParse}
	case has("flash"):
		return []string{TierCodeGen, TierLogParse}
	case has("opus", "gpt-4.5", "gpt-5", "-pro", "-r1") || strings.HasPrefix(m, "o1") || strings.HasPrefix(m, "o3"):
		return []string{TierReasoning}
	case has("sonnet", "gpt-4o", "gpt-4-turbo"):
		return []string{TierReasoning, TierCodeGen}
	case has("coder", "codellama", "starcoder", "llama", "qwen"):
		return []string{TierCodeGen}
	}
	return []string{TierCodeGen}
}

// providerKey folds provider aliases so "chatgpt/gpt-4o" and "openai/gpt-4o" compare equal.
func providerKey(p string) string {
	switch strings.ToLower(p) {
	case "chatgpt", "openai":
		return "openai"
	case "anthropic", "claude":
		return "claude"
	case "gemini", "google", "antigravity":
		return "antigravity"
	case "ollama", "vllm", "opencode":
		return "opencode"
	}
	return strings.ToLower(p)
}

// splitModel parses "provider/model"; a bare model id belongs to Claude (see ClientFactory.GetClient).
func splitModel(s string) (provider, model string) {
	if i := strings.IndexByte(s, '/'); i > 0 {
		return s[:i], s[i+1:]
	}
	return "claude", s
}

// SameModel reports whether two routable model strings name the same provider model.
func SameModel(a, b string) bool {
	pa, ma := splitModel(a)
	pb, mb := splitModel(b)
	return providerKey(pa) == providerKey(pb) && strings.EqualFold(ma, mb)
}

func enabledItems(chain []PriorityModelItem) []PriorityModelItem {
	var out []PriorityModelItem
	for _, item := range chain {
		if item.Enabled {
			out = append(out, item)
		}
	}
	return out
}

// usableItems drops items whose provider is known to be unusable. When that would leave nothing,
// it returns every item and degraded=true: trying a possibly-broken provider beats refusing to run.
func (r *Router) usableItems(items []PriorityModelItem) (out []PriorityModelItem, degraded bool) {
	if r.available == nil {
		return items, false
	}
	for _, item := range items {
		if r.available(item.Provider) {
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return items, len(items) > 0
	}
	return out, false
}

// chainModels is the fallback chain for items, in the given order.
func chainModels(items []PriorityModelItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.FullModel())
	}
	return out
}

// withPrimary puts primary first, then the pool's other candidates in rank order.
func withPrimary(primary string, candidates []PriorityModelItem) []string {
	out := []string{primary}
	for _, item := range candidates {
		if !SameModel(item.FullModel(), primary) {
			out = append(out, item.FullModel())
		}
	}
	return out
}

func rankOf(chain []PriorityModelItem, model string) int {
	for i, item := range chain {
		if SameModel(item.FullModel(), model) {
			return i + 1
		}
	}
	return 0
}

// bindToPool makes a tier recommendation (d.Model / d.Tier) obey the operator's chain: the
// recommended model is used only when it is enabled and usable; otherwise the highest-ranked
// usable item tagged for the tier (or, failing that, rank #1) runs and the recommendation becomes
// a suggestion. Fallbacks always come from the chain. An empty chain leaves d unrestricted.
func (r *Router) bindToPool(d *RoutingDecision, chain []PriorityModelItem) {
	enabled := enabledItems(chain)
	if len(enabled) == 0 {
		if d.Model != "" {
			d.FallbackChain = []string{d.Model}
		}
		return
	}
	candidates, degraded := r.usableItems(enabled)
	recommended := d.Model

	pick, why := -1, ""
	for i, c := range candidates {
		if recommended != "" && SameModel(c.FullModel(), recommended) {
			pick = i
			break
		}
	}
	if pick < 0 && d.Tier != "" {
		for i, c := range candidates {
			if c.hasTier(d.Tier) {
				pick, why = i, fmt.Sprintf("highest-ranked %s model in your chain", d.Tier)
				break
			}
		}
	}
	if pick < 0 {
		pick, why = 0, "no chain model is tagged "+d.Tier+"; using the top of your chain"
	}

	chosen := candidates[pick].FullModel()
	d.Model = chosen
	d.FallbackChain = withPrimary(chosen, candidates)
	if degraded {
		d.Reasoning += " | Warning: no provider in your chain is verified as connected; trying them anyway"
	}
	if recommended == "" || SameModel(recommended, chosen) {
		return
	}
	d.Suggestion = r.suggest(recommended, d.Tier, chain)
	d.Reasoning += fmt.Sprintf(" | Chain: recommended %s is not usable (%s); using %s (rank #%d, %s)",
		recommended, d.Suggestion.Action, chosen, rankOf(chain, chosen), why)
}

func (r *Router) suggest(model, tier string, chain []PriorityModelItem) *ModelSuggestion {
	s := &ModelSuggestion{Model: model, Tier: tier, Action: SuggestAdd,
		Reason: fmt.Sprintf("Best practice recommends %s for %s work. Add it to your chain to allow it.", model, tierLabel(tier))}
	for _, item := range chain {
		if !SameModel(item.FullModel(), model) {
			continue
		}
		if !item.Enabled {
			s.Action = SuggestEnable
			s.Reason = fmt.Sprintf("Best practice recommends %s for %s work. It is in your chain but disabled.", model, tierLabel(tier))
		} else {
			s.Action = SuggestConnect
			s.Reason = fmt.Sprintf("Best practice recommends %s, but provider %q is not connected.", model, item.Provider)
		}
		break
	}
	return s
}

func tierLabel(tier string) string {
	switch tier {
	case TierReasoning:
		return "reasoning (tier 1)"
	case TierCodeGen:
		return "code generation (tier 2)"
	case TierLogParse:
		return "log parsing (tier 3)"
	}
	return "this"
}
