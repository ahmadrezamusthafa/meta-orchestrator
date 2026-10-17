package packages

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

// The plan* functions validate an action before a job exists, so the API can answer 400/501
// immediately, and return what the job will run.

// planInstall checks that version of s can be installed here. inChain names packages a setup
// chain installs first: a tool they provide (npm, go) need not be on PATH yet.
func (m *Manager) planInstall(s *Spec, version string, inChain map[string]bool) (func(*jobRun) error, error) {
	chainProvides := false
	for _, r := range s.Requires {
		chainProvides = chainProvides || inChain[r]
	}
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		if !chainProvides {
			tool, why := m.managerTool(s)
			if err := m.needTool(tool, why); err != nil {
				return nil, err
			}
		}
		return func(r *jobRun) error { return m.installManaged(r, version) }, nil
	case KindNPXInstaller:
		if !chainProvides {
			if err := m.needTool("npx", "install Node.js first (the node package)"); err != nil {
				return nil, err
			}
		}
		return func(r *jobRun) error { return m.runInstaller(r, version) }, nil
	case KindClaudePlugin:
		if version != "" {
			return nil, ErrVersionNotPinnable
		}
		if !chainProvides {
			if err := m.needTool("claude", "install the Claude Code CLI first (the claude-code package)"); err != nil {
				return nil, err
			}
		}
		return func(r *jobRun) error {
			return r.run([]string{"claude", "plugin", "install", s.Package, "--scope", "user"}, nil, m.opts.UserHome)
		}, nil
	}
	// system
	if version != "" {
		return nil, ErrVersionNotPinnable
	}
	p := m.systemInstallPlan(s)
	if p.argv == nil {
		return nil, &ManualError{Reason: fmt.Sprintf("The orchestrator cannot install %s unattended on this machine; run one of the commands in your terminal.", s.Name), Manual: p.manual}
	}
	argv := p.argv
	return func(r *jobRun) error { return r.run(argv, nil, m.opts.Home) }, nil
}

func (m *Manager) planUpdate(s *Spec) (func(*jobRun) error, error) {
	switch s.Kind {
	case KindNPMCLI, KindGoCLI, KindNPXInstaller:
		return m.planInstall(s, "", nil)
	case KindClaudePlugin:
		if m.detectPlugin(s).status != StatusInstalled {
			return nil, ErrNotInstalled
		}
		if err := m.needTool("claude", "install the Claude Code CLI first"); err != nil {
			return nil, err
		}
		return func(r *jobRun) error {
			if _, err := m.takeSnapshot(r, "before update"); err != nil {
				return err
			}
			m.pruneSnapshots(r)
			if err := r.run([]string{"claude", "plugin", "update", s.Package}, nil, m.opts.UserHome); err != nil {
				return err
			}
			r.message = s.Name + " is updated. Restart Claude Code to load it."
			return nil
		}, nil
	}
	f := m.brewFormula(s)
	if f == "" || !brewAvailable() {
		return nil, m.systemManualError(s, "update")
	}
	return func(r *jobRun) error { return m.brewCommand(r, "upgrade", f) }, nil
}

// planRollback resolves the target version: the requested one, or the previous kept version /
// newest snapshot of another version.
func (m *Manager) planRollback(s *Spec, version string) (string, func(*jobRun) error, error) {
	st := m.store.get(s.ID)
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		target := version
		if target == "" {
			target = st.previousVersion()
		}
		if target == "" {
			return "", nil, ErrNothingToRollback
		}
		if st.kept(target) == nil || !m.binariesPresent(s, target) {
			tool, why := m.managerTool(s)
			if err := m.needTool(tool, why); err != nil {
				return "", nil, err
			}
		}
		return target, func(r *jobRun) error { return m.rollbackManaged(r, target) }, nil
	case KindNPXInstaller, KindClaudePlugin:
		current := m.detect(s).version
		sn := st.snapshotFor(version, current)
		if sn == nil {
			if version == "" {
				return "", nil, ErrNothingToRollback
			}
			if s.Kind == KindClaudePlugin {
				return "", nil, fmt.Errorf("%w: no snapshot of version %s is kept, and Claude Code cannot install a chosen plugin version", ErrNothingToRollback, version)
			}
			if err := m.needTool("npx", "install Node.js first (the node package)"); err != nil {
				return "", nil, err
			}
			return version, func(r *jobRun) error {
				r.log("No snapshot of %s is kept; installing it.", version)
				return m.runInstaller(r, version)
			}, nil
		}
		snap := *sn
		return snap.Version, func(r *jobRun) error { return m.restore(r, snap) }, nil
	}
	return "", nil, ErrRollbackUnsupported
}

func (m *Manager) planUninstall(s *Spec) (func(*jobRun) error, error) {
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		if len(m.store.get(s.ID).Versions) == 0 && !m.shimOwned(s, filepath.Join(m.BinDir(), s.Binaries[0])) {
			return nil, ErrNothingManaged
		}
		return m.uninstallManaged, nil
	case KindNPXInstaller:
		if m.detectInstaller(s).status == StatusNotInstalled {
			return nil, ErrNotInstalled
		}
		return m.uninstallInstaller, nil
	case KindClaudePlugin:
		if m.detectPlugin(s).status != StatusInstalled {
			return nil, ErrNotInstalled
		}
		if err := m.needTool("claude", "install the Claude Code CLI first"); err != nil {
			return nil, err
		}
		return func(r *jobRun) error {
			if _, err := m.takeSnapshot(r, "before uninstall"); err != nil {
				return err
			}
			m.pruneSnapshots(r)
			if err := r.run([]string{"claude", "plugin", "uninstall", s.Package, "--scope", "user"}, nil, m.opts.UserHome); err != nil {
				return err
			}
			r.message = s.Name + " is uninstalled (a snapshot was kept). Restart Claude Code."
			return nil
		}, nil
	}
	if m.detectSystem(s).status == StatusNotInstalled {
		return nil, ErrNotInstalled
	}
	f := m.brewFormula(s)
	if f == "" || !brewAvailable() {
		return nil, m.systemManualError(s, "uninstall")
	}
	return func(r *jobRun) error { return m.brewCommand(r, "uninstall", f) }, nil
}

// actions computes what the UI may offer for s right now.
func (m *Manager) actions(s *Spec, st PackageState, d detection) (Actions, ActionNotes, *Manual) {
	var a Actions
	var n ActionNotes
	var manual *Manual
	installed := d.status == StatusInstalled
	has := func(bin string) bool { _, err := exec.LookPath(bin); return err == nil }
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		tool, why := m.managerTool(s)
		ok := has(tool)
		a.Install, a.PickVersion = ok, ok
		a.Update = ok && (d.managed || installed)
		a.Rollback = d.managed && st.previousVersion() != ""
		a.Uninstall = len(st.Versions) > 0
		if !ok {
			n.Install = fmt.Sprintf("Needs %s: %s.", tool, why)
		} else {
			n.Install = fmt.Sprintf("Installs a managed copy under %s and links it from %s.", m.packageDir(s.ID), m.BinDir())
		}
		if !d.managed && d.systemCopy != "" {
			n.Update = "Installs a managed copy that takes precedence over the system copy at " + d.systemCopy + "."
		}
		if a.Rollback {
			n.Rollback = "Switches back to " + st.previousVersion() + " instantly (it is kept on disk)."
		} else {
			n.Rollback = "No earlier managed version is kept yet."
		}
		n.Uninstall = "Removes managed versions and shims only; a system copy is never touched."
	case KindNPXInstaller:
		ok := has("npx")
		a.Install, a.PickVersion = ok, ok
		a.Update = ok && installed
		a.Rollback = len(st.Snapshots) > 0
		a.Uninstall = installed
		if !ok {
			n.Install = "Needs npx: install Node.js first (the node package)."
		}
		n.Update = "A snapshot of the current files is taken first."
		if a.Rollback {
			n.Rollback = "Restores a snapshot offline; the current files are snapshotted first."
		} else {
			n.Rollback = "No snapshot exists yet; one is taken before every change."
		}
		n.Uninstall = "Takes a snapshot, then removes the files."
	case KindClaudePlugin:
		ok := has("claude")
		a.Install = ok && !installed
		a.Update = ok && installed
		a.Rollback = len(st.Snapshots) > 0
		a.Uninstall = ok && installed
		if !ok {
			n.Install = "Needs the Claude Code CLI (the claude-code package)."
		}
		n.Update = "Claude Code installs the plugin's latest version only (no version pinning); a snapshot is taken first. Restart Claude Code afterwards."
		n.Rollback = "Best effort: restores the plugin's snapshotted cache folder and rewrites its entry in Claude Code's installed_plugins.json (a .bak copy is kept). Restart Claude Code afterwards."
		if !a.Rollback {
			n.Rollback = "No snapshot exists yet; one is taken before every update or uninstall. " + n.Rollback
		}
		n.Uninstall = "Takes a snapshot, then runs `claude plugin uninstall`."
	default:
		p := m.systemInstallPlan(s)
		formula := m.brewFormula(s)
		brew := formula != "" && brewAvailable()
		a.Install = p.argv != nil && !installed
		a.Update = installed && brew
		a.Uninstall = installed && brew
		n.Rollback = systemRollbackNote
		if p.argv != nil {
			n.Install = "Runs `" + joinArgv(p.argv) + "`."
		} else {
			n.Install = "Needs your terminal on this machine; see the commands."
			if !installed {
				mm := p.manual
				manual = &mm
			}
		}
		if brew {
			n.Update = "Runs `brew upgrade " + formula + "`."
			n.Uninstall = "Runs `brew uninstall " + formula + "`; it fails if this copy was not installed by Homebrew."
		} else {
			n.Update = "Update it with the package manager that installed it."
			n.Uninstall = "Uninstall it with the package manager that installed it."
		}
	}
	return a, n, manual
}
