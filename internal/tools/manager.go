// Package tools reports and installs the command-line tools the orchestrator itself relies on
// (configs/tools.json). Status comes from running each tool's health check; installs go through
// internal/toolchain when the tool is in its catalog, and are left to the operator otherwise.
package tools

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/registry"
	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

// Tool states.
const (
	StatusInstalled    = "INSTALLED"
	StatusNotInstalled = "NOT_INSTALLED"
	StatusError        = "ERROR" // present, but the health check failed
)

const healthCheckTimeout = 20 * time.Second

// ToolStatus is a manifest entry with what this machine has of it.
type ToolStatus struct {
	ID               string                 `json:"id"`
	Name             string                 `json:"name"`
	Category         string                 `json:"category"`
	Description      string                 `json:"description"`
	Status           string                 `json:"status"`
	InstalledVersion string                 `json:"installed_version,omitempty"`
	Path             string                 `json:"path,omitempty"`
	Problem          string                 `json:"problem,omitempty"`
	HealthCheckCmd   string                 `json:"health_check_cmd"`
	RequiredRuntimes map[string]string      `json:"required_runtimes,omitempty"`
	ToolchainID      string                 `json:"toolchain_id,omitempty"`
	Install          registry.ManualInstall `json:"install"`
	Job              *toolchain.Job         `json:"job,omitempty"` // latest toolchain install
	CheckedAt        time.Time              `json:"checked_at"`
}

// ManualInstallError means the orchestrator cannot install the tool unattended; Install says how.
type ManualInstallError struct {
	ToolID  string
	Reason  string
	Install registry.ManualInstall
}

func (e *ManualInstallError) Error() string { return e.Reason }

// Errors returned by Install.
var (
	ErrUnknownTool       = errors.New("unknown tool")
	ErrInvalidVersion    = errors.New("invalid version: use letters, digits, '.', '-', '+' or '_' only")
	ErrVersionNotPinable = errors.New("installing a specific version is not supported; the package manager installs its current release")
)

var versionRe = regexp.MustCompile(`\d+\.\d+(?:\.\d+)?`)
var safeVersionRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)

// ValidateVersion rejects anything that is not a plain version label (no path separators, no "..").
func ValidateVersion(v string) error {
	if v == "" {
		return nil
	}
	if !safeVersionRe.MatchString(v) || strings.Contains(v, "..") {
		return ErrInvalidVersion
	}
	return nil
}

// ParseVersion extracts the first dotted version number from a tool's version output.
func ParseVersion(out string) string {
	return versionRe.FindString(out)
}

// ToolManager reports and installs the manifest's tools on this machine.
type ToolManager struct {
	reg *registry.ToolRegistry
	tc  *toolchain.Toolchain
}

// NewToolManager checks and installs reg's tools with tc.
func NewToolManager(reg *registry.ToolRegistry, tc *toolchain.Toolchain) *ToolManager {
	if reg == nil {
		reg = registry.NewToolRegistry()
	}
	return &ToolManager{reg: reg, tc: tc}
}

// Load reads the manifest at path. On error it still returns a usable, empty manager.
func Load(path string, tc *toolchain.Toolchain) (*ToolManager, error) {
	reg := registry.NewToolRegistry()
	err := reg.LoadFromFile(path)
	return NewToolManager(reg, tc), err
}

// Count is the number of tools in the manifest.
func (tm *ToolManager) Count() int { return len(tm.reg.ListPackages()) }

// checkTool is the toolchain description of a manifest entry's health check. The "hub:" prefix
// keeps its cached answer apart from a catalog tool of the same id.
func checkTool(p *registry.ToolPackageDefinition) toolchain.Tool {
	argv := p.HealthCheckArgv()
	return toolchain.Tool{ID: "hub:" + p.ID, Name: p.Name, Binary: argv[0], VersionArgs: argv[1:], Timeout: healthCheckTimeout}
}

// List runs (or reuses the cached result of) every tool's health check, in parallel.
func (tm *ToolManager) List() []ToolStatus {
	pkgs := tm.reg.ListPackages()
	checks := make([]toolchain.Tool, len(pkgs))
	for i, p := range pkgs {
		checks[i] = checkTool(p)
	}
	statuses := tm.tc.Checker.CheckAll(checks)
	out := make([]ToolStatus, 0, len(pkgs))
	for i, p := range pkgs {
		out = append(out, tm.status(p, statuses[checks[i].ID]))
	}
	return out
}

// Get reports one tool.
func (tm *ToolManager) Get(id string) (ToolStatus, error) {
	p, err := tm.reg.GetPackage(id)
	if err != nil {
		return ToolStatus{}, ErrUnknownTool
	}
	return tm.status(p, tm.tc.Checker.Check(checkTool(p))), nil
}

// Recheck forgets cached health checks, e.g. after the operator installed a tool themselves.
func (tm *ToolManager) Recheck() {
	toolchain.EnsurePath()
	tm.tc.Checker.Invalidate()
}

func (tm *ToolManager) status(p *registry.ToolPackageDefinition, s toolchain.Status) ToolStatus {
	ts := ToolStatus{ID: p.ID, Name: p.Name, Category: p.Category, Description: p.Description, Status: StatusNotInstalled,
		Path: s.Path, Problem: s.Problem, HealthCheckCmd: p.HealthCheckCmd, RequiredRuntimes: p.RequiredRuntimes,
		ToolchainID: p.ToolchainID, Install: p.Install, CheckedAt: s.CheckedAt}
	switch {
	case s.Installed:
		ts.Status, ts.InstalledVersion = StatusInstalled, ParseVersion(s.Version)
	case s.Problem != "":
		ts.Status = StatusError
	}
	if p.ToolchainID != "" && tm.tc.Installer != nil {
		ts.Job = tm.tc.Installer.Latest(p.ToolchainID)
	}
	return ts
}

// Install starts an unattended install through the toolchain catalog. A tool the catalog does
// not know, or whose methods all need the operator's terminal here, returns *ManualInstallError.
func (tm *ToolManager) Install(id, version string) (*toolchain.Job, error) {
	p, err := tm.reg.GetPackage(id)
	if err != nil {
		return nil, ErrUnknownTool
	}
	if err := ValidateVersion(version); err != nil {
		return nil, err
	}
	if version != "" {
		return nil, ErrVersionNotPinable
	}
	if p.ToolchainID == "" {
		return nil, &ManualInstallError{ToolID: id, Install: p.Install,
			Reason: fmt.Sprintf("The orchestrator has no unattended installer for %s; install it from your terminal.", p.Name)}
	}
	rep := tm.tc.Report([]toolchain.Requirement{{Tool: p.ToolchainID, Required: true}}, false)
	if len(rep.Items) == 0 {
		return nil, &ManualInstallError{ToolID: id, Install: p.Install,
			Reason: fmt.Sprintf("toolchain catalog has no entry %q", p.ToolchainID)}
	}
	item := rep.Items[0]
	manual := p.Install
	manual.Commands = append([]string(nil), p.Install.Commands...)
	if manual.Docs == "" {
		manual.Docs = item.Docs
	}
	for _, m := range item.Methods {
		if m.Available {
			manual.Commands = append(manual.Commands, m.Command)
		}
	}
	if item.Recommended < 0 {
		return nil, &ManualInstallError{ToolID: id, Install: manual,
			Reason: fmt.Sprintf("No package manager on this machine can install %s; see the docs.", p.Name)}
	}
	for _, m := range item.Methods {
		if m.Index == item.Recommended && !(m.Auto && m.Available) {
			if m.Note != "" {
				manual.Note = m.Note
			}
			return nil, &ManualInstallError{ToolID: id, Install: manual,
				Reason: fmt.Sprintf("Installing %s needs your terminal (sudo, a password or a GUI).", p.Name)}
		}
	}
	return tm.tc.Installer.Start(p.ToolchainID, item.Recommended)
}
