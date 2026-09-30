package uat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/atdd"
)

// App is an application a tester works in during UAT. Web apps are replayed in a browser for
// screenshots; API apps are checked by an engineer with the listed tools.
type App struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"` // "web" or "api"
	Audience  string   `json:"audience,omitempty"`
	Summary   string   `json:"summary,omitempty"`
	SignIn    []string `json:"sign_in,omitempty"`    // how a tester gets in, step by step
	Flags     string   `json:"flags,omitempty"`      // where feature flags for this app are toggled
	Keywords  []string `json:"keywords,omitempty"`   // words in a test case that place it in this app
	RepoHints []string `json:"repo_hints,omitempty"` // repository names/folders that belong to this app
	Repos     []string `json:"repos,omitempty"`      // assigned repositories resolved to this app
	// Per-task environment, set by the operator.
	BaseURL      string `json:"base_url,omitempty"`
	StorageState string `json:"storage_state,omitempty"`
}

// IsWeb reports whether the app has screens to capture.
func (a App) IsWeb() bool { return a.Kind != "api" }

// DefaultApps describes the Mekari billing applications. URLs are never assumed: the operator
// sets each environment per task. Teams can override or add apps in .sdlc/uat_apps.json.
var DefaultApps = []App{
	{
		ID: "subscription_backyard", Name: "Subscription Backyard", Kind: "web",
		Audience: "Billing operations, finance and sales admins",
		Summary:  "The billing microfrontend inside Mekari Backyard (billing-internal-fe). It reads and changes billing data through the billing GraphQL API.",
		SignIn: []string{
			"Open the Backyard URL for this environment in Chrome.",
			"Sign in with your UAT test account (never a personal production account).",
			"From the Backyard navigation, open **Subscription Backyard**.",
		},
		Flags:     "Backyard features are gated in Flagsmith (flags usually start with `rls_`); an engineer enables them for your test user before you start.",
		Keywords:  []string{"backyard", "subscription backyard", "bif", "internal-fe", "internal fe"},
		RepoHints: []string{"billing-internal-fe", "billing-frontend", "backyard"},
	},
	{
		ID: "billing_dashboard", Name: "Billing Dashboard", Kind: "web",
		Audience: "Finance and sales operations",
		Summary:  "The internal billing dashboard served by the billing application (Vue screens inside billing/).",
		SignIn: []string{
			"Open the Billing Dashboard URL for this environment in Chrome.",
			"Sign in with your UAT operations test account.",
		},
		Flags:     "Billing features are gated with Flipper feature flags; an engineer enables them in the Flipper admin before you start.",
		Keywords:  []string{"billing dashboard", "dashboard", "fe-billing"},
		RepoHints: []string{"billing-dashboard", "billing"},
	},
	{
		ID: "billing_api", Name: "Billing API (backend)", Kind: "api",
		Audience: "Engineers and QA",
		Summary:  "The billing backend (billing/, Rails): REST and GraphQL endpoints, background jobs and data. These checks have no screen.",
		SignIn: []string{
			"Use the API client your team uses for this environment (Postman collection or curl).",
			"Authenticate with a test API token issued for the UAT environment — never a production credential.",
		},
		Flags:     "Enable the Flipper flag named in the case through the Flipper admin.",
		Keywords:  []string{"api", "graphql", "endpoint", "sidekiq", "worker", "rails console"},
		RepoHints: []string{"billing-dashboard", "billing"},
	},
}

// LoadApps returns the default apps with overrides from {root}/.sdlc/uat_apps.json merged in
// by ID (a new ID adds an app).
func LoadApps(root string) []App {
	apps := append([]App(nil), DefaultApps...)
	raw, err := os.ReadFile(filepath.Join(root, ".sdlc", "uat_apps.json"))
	if err != nil {
		return apps
	}
	var custom []App
	if json.Unmarshal(raw, &custom) != nil {
		return apps
	}
	for _, c := range custom {
		found := false
		for i := range apps {
			if apps[i].ID == c.ID {
				mergeApp(&apps[i], c)
				found = true
			}
		}
		if !found && c.ID != "" {
			apps = append(apps, c)
		}
	}
	return apps
}

func mergeApp(dst *App, src App) {
	if src.Name != "" {
		dst.Name = src.Name
	}
	if src.Kind != "" {
		dst.Kind = src.Kind
	}
	if src.Audience != "" {
		dst.Audience = src.Audience
	}
	if src.Summary != "" {
		dst.Summary = src.Summary
	}
	if len(src.SignIn) > 0 {
		dst.SignIn = src.SignIn
	}
	if src.Flags != "" {
		dst.Flags = src.Flags
	}
	if len(src.Keywords) > 0 {
		dst.Keywords = src.Keywords
	}
	if len(src.RepoHints) > 0 {
		dst.RepoHints = src.RepoHints
	}
}

// RepoRef is an assigned repository: its registered name and source folder.
type RepoRef struct{ Name, Path string }

func repoMatches(a App, r RepoRef) bool {
	base := strings.ToLower(filepath.Base(r.Path))
	name := strings.ToLower(r.Name)
	for _, h := range a.RepoHints {
		h = strings.ToLower(h)
		if name == h || base == h {
			return true
		}
	}
	return false
}

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// AppsForRepos picks the apps a task touches from its assigned repositories. A repository no
// profile claims becomes its own web app, so the guide still has a place for its cases.
func AppsForRepos(all []App, repos []RepoRef) []App {
	var out []App
	index := map[string]int{}
	for _, r := range repos {
		matched := false
		for _, a := range all {
			if !repoMatches(a, r) {
				continue
			}
			matched = true
			if i, ok := index[a.ID]; ok {
				out[i].Repos = append(out[i].Repos, r.Name)
				continue
			}
			a.Repos = []string{r.Name}
			index[a.ID] = len(out)
			out = append(out, a)
		}
		if !matched && !strings.Contains(strings.ToLower(r.Name), "artifact") && !strings.Contains(strings.ToLower(r.Name), "automation") {
			id := strings.Trim(nonID.ReplaceAllString(strings.ToLower(r.Name), "_"), "_")
			if _, ok := index[id]; !ok && id != "" {
				index[id] = len(out)
				out = append(out, App{ID: id, Name: r.Name, Kind: "web", Repos: []string{r.Name},
					SignIn: []string{"Open the " + r.Name + " URL for this environment and sign in with your UAT test account."}})
			}
		}
	}
	return out
}

// AppFor decides which app an ATDD case runs in: the sheet's Repository column, then the
// platform (API cases go to an API app), then keywords in the case text, then the first web app.
func AppFor(c atdd.Case, apps []App) string {
	if len(apps) == 0 {
		return ""
	}
	if repo := strings.ToLower(strings.TrimSpace(c.Repository)); repo != "" {
		for _, a := range apps {
			if (a.IsWeb() || c.IsAPI()) && (containsFold(a.RepoHints, repo) || containsFold(a.Repos, repo) || strings.EqualFold(a.ID, repo)) {
				return a.ID
			}
		}
	}
	if c.IsAPI() {
		for _, a := range apps {
			if !a.IsWeb() {
				return a.ID
			}
		}
	}
	text := strings.ToLower(strings.Join(append(append([]string{c.Title}, c.Preconditions...), c.Steps...), " "))
	best, bestLen := "", 0
	for _, a := range apps {
		if !a.IsWeb() && !c.IsAPI() {
			continue
		}
		for _, k := range a.Keywords {
			if strings.Contains(text, strings.ToLower(k)) && len(k) > bestLen {
				best, bestLen = a.ID, len(k) // the most specific keyword wins ("subscription backyard" over "dashboard")
			}
		}
	}
	if best != "" {
		return best
	}
	for _, a := range apps {
		if a.IsWeb() {
			return a.ID
		}
	}
	return apps[0].ID
}

func containsFold(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(x, v) {
			return true
		}
	}
	return false
}
