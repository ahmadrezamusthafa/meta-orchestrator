package api

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/router"
)

// ProviderQuota is an operator-set spending/usage cap for one provider, measured month-to-date.
// Zero means "no limit". Providers do not expose subscription quotas, so these are local budgets.
type ProviderQuota struct {
	MonthlyTokenLimit   int64   `json:"monthly_token_limit"`
	MonthlyCostLimitUSD float64 `json:"monthly_cost_limit_usd"`
}

// persistedProviderAuth is the on-disk form of ProviderAuthState. Unlike the API DTO it keeps the
// credentials, so a daemon restart does not ask the operator to paste them again.
type persistedProviderAuth struct {
	AuthMethod   string    `json:"auth_method"`
	APIKey       string    `json:"api_key,omitempty"`
	SessionToken string    `json:"session_token,omitempty"`
	MaskedKey    string    `json:"masked_key,omitempty"`
	SavedAt      time.Time `json:"saved_at"`
}

type providerStateFile struct {
	Auth          map[string]persistedProviderAuth `json:"auth"`
	DefaultModels map[string]string                `json:"default_models,omitempty"`
	Quotas        map[string]ProviderQuota         `json:"quotas,omitempty"`
	Router        *router.RouterSettings           `json:"router,omitempty"`
}

// routerSettingsState is the saved routing mode and priority chain; guarded by providerAuthStore.mu.
var routerSettingsState *router.RouterSettings

// providerQuotas holds the per-provider budgets; guarded by providerAuthStore.mu.
var providerQuotas = map[string]ProviderQuota{}

// providerStatePath is where provider auth, default models and quotas persist. Empty keeps them in
// memory only (tests). Set once from RouterConfig.ProviderStatePath.
var providerStatePath string

// loadProviderState restores auth, default models and quotas saved by a previous daemon run.
func loadProviderState(path string) {
	providerStatePath = path
	providerAuthStore.mu.Lock()
	routerSettingsState = nil // never inherit another router's settings
	providerAuthStore.mu.Unlock()
	if path == "" {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Printf("[Providers] State not loaded from %s: %v\n", path, err)
		}
		return
	}
	var f providerStateFile
	if err := json.Unmarshal(data, &f); err != nil {
		fmt.Printf("[Providers] State file %s is invalid: %v\n", path, err)
		return
	}

	providerAuthStore.mu.Lock()
	for id, a := range f.Auth {
		providerAuthStore.state[id] = &ProviderAuthState{AuthMethod: a.AuthMethod, APIKey: a.APIKey,
			SessionToken: a.SessionToken, MaskedKey: a.MaskedKey, SavedAt: a.SavedAt}
	}
	for id, q := range f.Quotas {
		providerQuotas[id] = q
	}
	routerSettingsState = f.Router
	for i := range defaultProviders {
		if m := f.DefaultModels[defaultProviders[i].ID]; m != "" {
			defaultProviders[i].DefaultModel = m
		}
	}
	providerAuthStore.mu.Unlock()
}

// saveProviderStateLocked writes the provider state file. Caller holds providerAuthStore.mu.
// The file carries live credentials, so it is written owner-only (0600) and atomically.
func saveProviderStateLocked() error {
	if providerStatePath == "" {
		return nil
	}
	f := providerStateFile{Auth: map[string]persistedProviderAuth{}, DefaultModels: map[string]string{}, Quotas: providerQuotas,
		Router: routerSettingsState}
	for id, s := range providerAuthStore.state {
		if s == nil {
			continue
		}
		f.Auth[id] = persistedProviderAuth{AuthMethod: s.AuthMethod, APIKey: s.APIKey,
			SessionToken: s.SessionToken, MaskedKey: s.MaskedKey, SavedAt: s.SavedAt}
	}
	for _, p := range defaultProviders {
		f.DefaultModels[p.ID] = p.DefaultModel
	}
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(providerStatePath), 0o700); err != nil {
		return err
	}
	tmp := providerStatePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, providerStatePath)
}

// savedRouterSettings returns the routing settings restored from disk, if any.
func savedRouterSettings() *router.RouterSettings {
	providerAuthStore.mu.RLock()
	defer providerAuthStore.mu.RUnlock()
	if routerSettingsState == nil {
		return nil
	}
	s := *routerSettingsState
	return &s
}

// saveRouterSettings persists the routing mode and priority chain across daemon restarts.
func saveRouterSettings(s router.RouterSettings) error {
	providerAuthStore.mu.Lock()
	defer providerAuthStore.mu.Unlock()
	routerSettingsState = &s
	return saveProviderStateLocked()
}
