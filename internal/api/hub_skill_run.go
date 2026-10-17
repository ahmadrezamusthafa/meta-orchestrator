package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/skills"
	"github.com/ahmadrezamusthafa/meta-orchestrator/pkg/types"
)

// A skill test run starts a command-based skill (a Claude skill with a command or entrypoint
// script, an MCP server, a superpower tool) once in the project root, as a stage role, so the
// operator can see it actually works before attaching it. Instruction-only skills have nothing to
// run and are refused.
const (
	skillRunMaxTimeout = 60 * time.Second
	skillRunMaxOutput  = 64 << 10
)

type skillRunRequest struct {
	Role       string                 `json:"role"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type skillRunResponse struct {
	Skill      string `json:"skill"`
	Role       string `json:"role"`
	OK         bool   `json:"ok"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output"`
	Error      string `json:"error,omitempty"`
	DurationMS int64  `json:"duration_ms"`
}

// handleSkillRun serves POST /api/v1/skills/{name}/run.
func (r *Router) handleSkillRun(w http.ResponseWriter, req *http.Request, name string) {
	if req.Method != http.MethodPost {
		r.writeError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var body skillRunRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.Role == "" {
		r.writeError(w, http.StatusBadRequest, `expected {"role": "<profile role>", "parameters": {…}}`)
		return
	}
	profile, err := r.profileRegistry().GetProfile(body.Role)
	if err != nil {
		r.writeError(w, http.StatusBadRequest, fmt.Sprintf("unknown role %q: use a role from configs/profiles.json", body.Role))
		return
	}
	skill, err := r.cfg.SkillResolver.ResolveSkill(name)
	if err != nil {
		r.writeError(w, http.StatusNotFound, "Skill not found: "+name)
		return
	}
	if !r.cfg.SkillResolver.IsSkillEnabled(name) {
		r.writeError(w, http.StatusConflict, "skill "+name+" is disabled")
		return
	}
	if r.profileRegistry().IsDenied(profile.Role, name) {
		r.writeError(w, http.StatusForbidden, fmt.Sprintf("the %s profile denies skill %s", profile.Name, name))
		return
	}
	if !skills.Runnable(skill) {
		r.writeError(w, http.StatusUnprocessableEntity,
			"skill "+name+" has no command to run; instruction skills take effect when attached to a stage")
		return
	}

	run := *skill
	if run.TimeoutSeconds <= 0 || time.Duration(run.TimeoutSeconds)*time.Second > skillRunMaxTimeout {
		run.TimeoutSeconds = int(skillRunMaxTimeout / time.Second)
	}
	executor := &skills.StandardSkillExecutor{MaxOutputBytes: skillRunMaxOutput}
	start := time.Now()
	res, err := executor.Execute(req.Context(), &run, &types.SkillExecutionRequest{
		CallerRole: profile.Role, Parameters: body.Parameters, WorkspacePath: r.cfg.RootDir}, nil)
	resp := skillRunResponse{Skill: name, Role: profile.Role, DurationMS: time.Since(start).Milliseconds()}
	if res != nil {
		resp.ExitCode, resp.Output = res.ExitCode, res.Stdout
	}
	if err != nil {
		resp.Error = err.Error()
		if res == nil {
			r.writeJSON(w, http.StatusForbidden, resp) // refused before starting (role not allowed)
			return
		}
	}
	resp.OK = err == nil && resp.ExitCode == 0
	r.writeJSON(w, http.StatusOK, resp)
}
