package api

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/hooks"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
)

type RegistryDTO struct {
	Skills   []SkillItemDTO           `json:"skills"`
	Prompts  []registry.PromptItemDTO `json:"prompts"`
	Hooks    []HookItemDTO            `json:"hooks"`
	Methods  []MethodItemDTO          `json:"methods"`
	Profiles []ProfileItemDTO         `json:"profiles"`
	// Problems lists registry files that exist but could not be read.
	Problems []string `json:"problems"`
}

type SkillItemDTO struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Source      string `json:"source"` // where the resolver found it: claude, project_local, mcp_connector, custom_dir, system, builtin
	Format      string `json:"format"` // claude, mcp, bmad, superpower, openai, native
	Description string `json:"description"`
	Location    string `json:"location,omitempty"`
	Enabled     bool   `json:"enabled"`
}

type HookItemDTO struct {
	ID      string `json:"id"`
	Event   string `json:"event"`
	Type    string `json:"type"`    // "SHELL", "DOCKER", "WEBHOOK"
	Command string `json:"command"` // webhook URLs are shown as scheme://host/… (their paths often hold tokens)
	Policy  string `json:"policy"`  // "BLOCK", "WARN"
	Source  string `json:"source"`  // ".sdlc/hooks.json" or ".sdlc/hooks/<event>.sh"
}

type MethodItemDTO struct {
	ID                       string   `json:"id"`
	Name                     string   `json:"name"`
	Description              string   `json:"description"`
	ExecutionPattern         string   `json:"execution_pattern"`
	AllowedRoles             []string `json:"allowed_roles"`
	MaxConsecutiveIterations int      `json:"max_consecutive_iterations"`
}

type ProfileItemDTO struct {
	registry.AgentProfile
	// Stages are the pipeline stages that run with this role.
	Stages []string `json:"stages"`
}

// handleRegistries: GET /api/v1/registries lists what the orchestrator actually has loaded: skills
// from the live resolver, prompt templates, lifecycle hooks (.sdlc/hooks.json and the convention
// scripts .sdlc/hooks/<event>.sh), methods (configs/methods.json) and stage roles.
func (r *Router) handleRegistries(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		r.writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	data := RegistryDTO{Skills: r.registrySkills(), Prompts: []registry.PromptItemDTO{}, Problems: []string{}}
	if r.cfg.PromptRegistry != nil {
		data.Prompts = r.cfg.PromptRegistry.ListPromptTemplates()
	}
	var problems []string
	data.Hooks, problems = r.registryHooks()
	data.Problems = append(data.Problems, problems...)
	data.Methods, problems = r.registryMethods()
	data.Problems = append(data.Problems, problems...)
	data.Profiles = r.registryProfiles()
	r.writeJSON(w, http.StatusOK, data)
}

func (r *Router) registrySkills() []SkillItemDTO {
	out := []SkillItemDTO{}
	if r.cfg.SkillResolver == nil {
		return out
	}
	list, err := r.cfg.SkillResolver.ListAllSkillsWithDetails(false)
	if err != nil {
		return out
	}
	for _, s := range list {
		out = append(out, SkillItemDTO{ID: s.Name, Name: s.Name, Source: s.SourceType, Format: string(s.SourceFormat),
			Description: s.Description, Location: s.SourceLocation, Enabled: s.Enabled})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// registryHooks reads the hooks the hook engine runs: .sdlc/hooks.json (a list of
// hooks.HookDefinition) and every convention script in .sdlc/hooks/.
func (r *Router) registryHooks() ([]HookItemDTO, []string) {
	out := []HookItemDTO{}
	var problems []string
	sdlc := filepath.Join(r.cfg.RootDir, ".sdlc")
	if data, err := os.ReadFile(filepath.Join(sdlc, "hooks.json")); err == nil {
		var defs []hooks.HookDefinition
		if err := json.Unmarshal(data, &defs); err != nil {
			problems = append(problems, ".sdlc/hooks.json: "+err.Error())
		}
		for _, d := range defs {
			policy := "WARN"
			if d.IsBlocking {
				policy = "BLOCK"
			}
			target := d.Target
			if d.Driver == hooks.HookDriverWebhook {
				target = redactURL(target)
			}
			out = append(out, HookItemDTO{ID: d.ID, Event: string(d.Event), Type: strings.ToUpper(string(d.Driver)),
				Command: target, Policy: policy, Source: ".sdlc/hooks.json"})
		}
	}
	scripts, _ := filepath.Glob(filepath.Join(sdlc, "hooks", "*.sh"))
	sort.Strings(scripts)
	for _, s := range scripts {
		event := strings.TrimSuffix(filepath.Base(s), ".sh")
		rel := filepath.Join(".sdlc", "hooks", filepath.Base(s))
		// The engine runs a convention script as a blocking shell hook of the event it is named after.
		out = append(out, HookItemDTO{ID: "convention-" + event, Event: event, Type: "SHELL", Command: rel, Policy: "BLOCK", Source: rel})
	}
	return out, problems
}

// redactURL keeps a webhook's scheme and host; paths and queries of webhook URLs often carry tokens.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(webhook URL hidden)"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

func (r *Router) registryMethods() ([]MethodItemDTO, []string) {
	out := []MethodItemDTO{}
	path := filepath.Join(r.cfg.RootDir, "configs", "methods.json")
	reg := registry.NewMethodRegistry()
	if err := reg.LoadFromFile(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return out, nil
		}
		return out, []string{"configs/methods.json: " + err.Error()}
	}
	// MethodRegistry has no listing; its ids come from the same file it just loaded.
	data, _ := os.ReadFile(path)
	var ids []struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(data, &ids)
	for _, it := range ids {
		m, err := reg.Get(it.ID)
		if err != nil {
			continue
		}
		out = append(out, MethodItemDTO{ID: m.ID, Name: m.Name, Description: m.Description, ExecutionPattern: m.ExecutionPattern,
			AllowedRoles: m.AllowedRoles, MaxConsecutiveIterations: m.MaxConsecutiveIterations})
	}
	return out, nil
}

func (r *Router) registryProfiles() []ProfileItemDTO {
	out := []ProfileItemDTO{}
	for _, p := range r.profileRegistry().ListProfiles() {
		item := ProfileItemDTO{AgentProfile: *p, Stages: []string{}}
		for _, st := range stagePipeline {
			if stageRoles[st] == p.Role {
				item.Stages = append(item.Stages, st)
			}
		}
		out = append(out, item)
	}
	return out
}
