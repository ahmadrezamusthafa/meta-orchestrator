// Package readiness defines the launch-readiness steps a new service or initiative goes through
// before production — check how it is released today, make releases automatic, watch the system
// and send alerts, check security automatically, set up the API for customers, and test with heavy
// traffic — and the built-in skill (SKILL.md playbook) that guides each step.
//
// Each step is a workflow stage that ends at an operator gate: the stage's gate checklist is what
// the operator confirms before approving it.
package readiness

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed skills
var skillFS embed.FS

// ToolNeed is a command-line tool a step runs (internal/toolchain catalog id). A required tool
// stops the stage until it is installed or the operator chooses to run without it; a recommended
// one is reported but never blocks.
type ToolNeed struct {
	ID       string
	Required bool
	Why      string
}

// Step is one launch-readiness stage.
type Step struct {
	ID   string
	Name string
	// Skill is the built-in playbook loaded into the stage brief.
	Skill string
	// Role is the agent profile role that performs the step.
	Role string
	// Code is true when the step changes repository files (it needs assigned repositories).
	Code bool
	// Goal is the stage instruction: what to do and the output it must produce.
	Goal string
	// Gate is what the operator confirms before approving the step.
	Gate []string
	// Tools are the tools the step runs on this machine.
	Tools []ToolNeed
}

// Steps are the launch-readiness stages in order.
var Steps = []Step{
	{
		ID: "release_audit", Name: "Check How We Release Today", Skill: "launch-release-audit", Role: "devops_superpower",
		Goal: "Audit how this service is released today, read-only: map the real path from a merged change to production " +
			"(build, tests, artifact, environments, deploy, rollback, approvals, secrets, database migrations) from the repositories " +
			"and their CI/CD configuration, score each area, and list the gaps the next readiness stages must close.",
		Gate: []string{
			"Every step of the release path is marked automated, manual or unknown, with the file it comes from",
			"Each scorecard area has a score and evidence",
			"Gaps are ranked and each names the readiness stage that closes it",
			"Unknowns are written as questions the operator can answer",
		},
	},
	{
		ID: "release_automation", Name: "Make Releases Automatic", Skill: "launch-release-automation", Role: "devops_superpower", Code: true,
		Goal: "Make releases automatic: build or harden the CI/CD pipeline as code in the repository — build once into an immutable " +
			"versioned artifact, blocking tests, automatic deploy to staging with smoke checks, an approval-gated promotion of the same " +
			"artifact to production, safe database migrations and a one-command rollback. Close the release audit's gaps.",
		Gate: []string{
			"The pipeline is code in the repository and was validated (linter or dry run named)",
			"Production promotion reuses the staging artifact and needs an approval",
			"Rollback is one documented command and migrations are backward compatible",
			"No secret values in files, and every secret the operator must create is listed",
		},
		Tools: []ToolNeed{
			{ID: "actionlint", Why: "validates GitHub Actions workflows"},
			{ID: "hadolint", Why: "lints the Dockerfile the pipeline builds"},
			{ID: "docker", Why: "builds the image locally to check the Dockerfile"},
		},
	},
	{
		ID: "observability_alerting", Name: "Watch the System & Send Alerts", Skill: "launch-observability", Role: "devops_superpower", Code: true,
		Goal: "Make the service observable: structured logs with personal data and secrets redacted, RED/USE metrics, traces and " +
			"health endpoints; SLIs and SLOs for the critical journeys; symptom-based alerts with a severity, route and runbook each; " +
			"and dashboards as code.",
		Gate: []string{
			"SLOs are defined for each critical journey (targets marked as assumptions until confirmed)",
			"Every alert has a severity, a route and a runbook",
			"Logs redact personal data and secrets, with a test proving it",
			"Liveness and readiness endpoints exist and are tested",
		},
		Tools: []ToolNeed{
			{ID: "promtool", Why: "validates and unit-tests the Prometheus alert rules"},
		},
	},
	{
		ID: "security_automation", Name: "Check Security Automatically", Skill: "launch-security-scanning", Role: "devops_superpower", Code: true,
		Goal: "Add automatic security checks to the pipeline — secret scanning, dependency/SCA, SAST, container and IaC scanning, " +
			"an SBOM, and a DAST baseline against staging — with a baseline and a triage/suppression policy, then review access " +
			"control, input validation, secrets management and personal-data handling in the code.",
		Gate: []string{
			"Each check runs in the pipeline and its blocking rule is stated",
			"Existing findings are triaged, and every suppression has a reason, owner and expiry",
			"Code review findings name a risk area and file:line",
			"Personal-data fields are inventoried with their protection",
		},
		Tools: []ToolNeed{
			{ID: "gitleaks", Required: true, Why: "secret scanning of the code and its history"},
			{ID: "trivy", Required: true, Why: "dependency, container image and IaC scanning"},
			{ID: "semgrep", Required: true, Why: "static analysis (SAST) baseline"},
			{ID: "osv-scanner", Why: "second opinion on lockfile vulnerabilities"},
			{ID: "govulncheck", Why: "reachable vulnerabilities in Go code"},
			{ID: "syft", Why: "software bill of materials (SBOM)"},
			{ID: "checkov", Why: "Terraform, Kubernetes and Helm misconfigurations"},
			{ID: "docker", Why: "runs the OWASP ZAP baseline scan against staging"},
		},
	},
	{
		ID: "customer_api_setup", Name: "Set Up the API for Customers", Skill: "launch-customer-api", Role: "lead_developer", Code: true,
		Goal: "Make the customer-facing API safe to expose: an OpenAPI contract as the source of truth, a versioning and deprecation " +
			"policy, per-client authentication and scopes, tenant isolation, rate limits, idempotency, one error format, gateway " +
			"configuration, contract tests and customer-facing docs.",
		Gate: []string{
			"The OpenAPI spec covers every public operation and is validated against the implementation in CI",
			"Auth, tenant isolation, rate limits and idempotency are tested",
			"Versioning and deprecation policy is written down",
			"Customer docs let a new client make a first successful call",
		},
		Tools: []ToolNeed{
			{ID: "spectral", Required: true, Why: "lints the OpenAPI contract"},
			{ID: "node", Why: "runs npx-based contract and docs tooling"},
		},
	},
	{
		ID: "load_resilience", Name: "Test Heavy Traffic & Prepare for Problems", Skill: "launch-load-resilience", Role: "devops_superpower", Code: true,
		Goal: "Load-test and prepare for incidents: load tests as code (baseline, stress, spike, soak) with thresholds from the SLOs, " +
			"run only against staging with the operator's approval; review timeouts, retries, circuit breakers, backpressure and " +
			"graceful shutdown; and write the failure-mode table, runbooks, rollback drill and capacity plan.",
		Gate: []string{
			"Load tests are code with SLO thresholds, and their results (or why they were not run) are reported",
			"The breaking point and how the service fails there are known",
			"Resilience gaps are listed with file:line and a fix",
			"Runbooks, a rollback drill and a capacity plan exist",
		},
		Tools: []ToolNeed{
			{ID: "k6", Required: true, Why: "runs the load tests and checks their SLO thresholds"},
			{ID: "docker", Why: "starts dependencies for a local or staging-like test run"},
		},
	},
}

// RequiredTools lists the ids of the tools a step cannot run its checks without.
func (s Step) RequiredTools() []string {
	var out []string
	for _, t := range s.Tools {
		if t.Required {
			out = append(out, t.ID)
		}
	}
	return out
}

// StepByID returns the readiness step with the given stage ID.
func StepByID(id string) (Step, bool) {
	for _, s := range Steps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}

// Instruction is the stage brief of a step: its goal, where to build from, and its gate checklist.
func (s Step) Instruction() string {
	var b strings.Builder
	b.WriteString(s.Goal)
	b.WriteString(" Build on the earlier stages of this task (this conversation, or their documents in the task's stages/ " +
		"folder) and follow the stage's built-in playbook skill for the method and the report headings. " +
		"Never print a secret or personal data you come across — give its file and line, masked.")
	b.WriteString("\n\nThe operator confirms this checklist before approving the stage; end your Summary with it, each item marked ✓, ✗ or n/a with a one-line reason:\n")
	for _, g := range s.Gate {
		b.WriteString("- " + g + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

// GateCriteria is the gate checklist as one line, for the workflow definition.
func (s Step) GateCriteria() string {
	return strings.Join(s.Gate, "; ")
}

// SkillNames lists the built-in readiness skills.
func SkillNames() []string {
	out := make([]string, len(Steps))
	for i, s := range Steps {
		out[i] = s.Skill
	}
	return out
}

// installedManifest records the digest of each file as last installed, so an upgrade replaces only
// files the operator has not edited.
const installedManifest = ".builtin.json"

// InstallSkills writes the built-in readiness skills into dir (one folder per skill). A new or
// unchanged file is written with the current version; a file the operator edited is left alone.
func InstallSkills(dir string) error {
	entries, err := fs.ReadDir(skillFS, "skills")
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := installSkill(e.Name(), filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func installSkill(name, target string) error {
	manifestPath := filepath.Join(target, installedManifest)
	installed := map[string]string{}
	if raw, err := os.ReadFile(manifestPath); err == nil {
		_ = json.Unmarshal(raw, &installed)
	}
	root := path.Join("skills", name)
	changed := false
	err := fs.WalkDir(skillFS, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		dst := filepath.Join(target, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := skillFS.ReadFile(p)
		if err != nil {
			return err
		}
		want := digest(data)
		if cur, err := os.ReadFile(dst); err == nil {
			if bytes.Equal(cur, data) {
				if installed[rel] != want {
					installed[rel], changed = want, true
				}
				return nil
			}
			if prev, ok := installed[rel]; !ok || prev != digest(cur) {
				return nil // edited by the operator: keep their version
			}
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			return err
		}
		installed[rel], changed = want, true
		return nil
	})
	if err != nil || !changed {
		return err
	}
	raw, err := json.MarshalIndent(installed, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, raw, 0o644)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
