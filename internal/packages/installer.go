package packages

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// npx-installer packages (BMAD, GSD) run their own installer, which writes into the operator's
// home folder. Every change is preceded by a snapshot of the package's footprint, and a rollback
// restores one.

var unsafeNameRe = regexp.MustCompile(`[^0-9A-Za-z.+_-]`)

// takeSnapshot archives what the package owns now. It returns nil when nothing is there.
func (m *Manager) takeSnapshot(r *jobRun, reason string) (*Snapshot, error) {
	s := r.spec
	var rels []string
	var entries []byte
	pluginPath := ""
	version := ""
	switch s.Kind {
	case KindClaudePlugin:
		e, err := m.pluginEntries(s.Package)
		if err != nil {
			return nil, err
		}
		if e == nil {
			r.log("Nothing to snapshot: %s is not in installed_plugins.json.", s.Package)
			return nil, nil
		}
		entries, version = e.raw, e.version
		if e.installPath != "" {
			rel, err := relInside(m.opts.UserHome, e.installPath)
			if err != nil {
				return nil, fmt.Errorf("plugin folder %s is outside the home folder; cannot snapshot it", e.installPath)
			}
			if _, err := os.Stat(e.installPath); err == nil {
				rels, pluginPath = []string{rel}, rel
			}
		}
	default:
		var err error
		rels, err = resolveFootprint(footprintPatterns(s, m.opts.UserHome, m.scopeDir(s)), m.opts.UserHome, m.scopeDir(s))
		if err != nil {
			return nil, err
		}
		if len(rels) == 0 {
			r.log("Nothing to snapshot: no files of %s exist yet.", s.Name)
			return nil, nil
		}
		if d := m.detectInstaller(s); d.version != VersionUnknown {
			version = d.version
		}
	}
	label := version
	if label == "" {
		label = VersionUnknown
	}
	now := time.Now().UTC()
	file := fmt.Sprintf("%s-%s.tar.gz", unsafeNameRe.ReplaceAllString(label, "_"), now.Format("20060102T150405.000000000Z"))
	dest := filepath.Join(m.snapshotDir(s.ID), file)
	r.log("Snapshotting %s (%s) to %s …", s.Name, label, dest)
	if err := writeSnapshot(dest, m.opts.UserHome, rels); err != nil {
		return nil, fmt.Errorf("snapshot: %w", err)
	}
	sn := Snapshot{Version: version, File: file, CreatedAt: now, Reason: reason, PluginEntries: entries, PluginPath: pluginPath}
	if err := m.store.update(s.ID, func(p *PackageState) { p.Snapshots = append(p.Snapshots, sn) }); err != nil {
		return nil, err
	}
	return &sn, nil
}

// pruneSnapshots keeps the newest keepSnapshots snapshots.
func (m *Manager) pruneSnapshots(r *jobRun) {
	var dropped []Snapshot
	_ = m.store.update(r.spec.ID, func(p *PackageState) {
		if over := len(p.Snapshots) - keepSnapshots; over > 0 {
			dropped = append(dropped, p.Snapshots[:over]...)
			p.Snapshots = append([]Snapshot(nil), p.Snapshots[over:]...)
		}
	})
	for _, sn := range dropped {
		_ = os.Remove(filepath.Join(m.snapshotDir(r.spec.ID), sn.File))
	}
}

func (m *Manager) installerArgs(s *Spec, args []string, version string) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		a = expand(a, m.opts.UserHome, m.scopeDir(s))
		out = append(out, strings.ReplaceAll(a, "{version}", version))
	}
	return out
}

// runInstaller installs version ("" = latest) with the package's own installer.
func (m *Manager) runInstaller(r *jobRun, version string) error {
	s := r.spec
	if version == "" {
		v, err := m.resolveLatest(r.ctx, s)
		if err != nil {
			return err
		}
		version = v
		r.setVersion(v)
		r.log("Latest release: %s", v)
	}
	installed := m.detectInstaller(s).status == StatusInstalled
	if _, err := m.takeSnapshot(r, "before "+map[bool]string{true: "update", false: "install"}[installed]+" to "+version); err != nil {
		return err
	}
	args := s.InstallArgs
	if installed && len(s.UpdateArgs) > 0 {
		args = s.UpdateArgs
	}
	dir := m.scopeDir(s)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	argv := append([]string{"npx", "-y", s.Package + "@" + version}, m.installerArgs(s, args, version)...)
	err := r.run(argv, nil, dir)
	m.pruneSnapshots(r)
	if err != nil {
		return err
	}
	d := m.detectInstaller(s)
	if d.status == StatusInstalled && d.version != version && d.version != VersionUnknown {
		r.log("Note: %s now reports version %s.", s.Name, d.version)
	}
	return m.store.update(s.ID, func(p *PackageState) { p.Active = d.version })
}

// restore replaces what is installed with snapshot sn, after snapshotting the current state.
func (m *Manager) restore(r *jobRun, sn Snapshot) error {
	s := r.spec
	src := filepath.Join(m.snapshotDir(s.ID), sn.File)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("snapshot file %s is missing", src)
	}
	if err := validateSnapshot(src, m.opts.UserHome); err != nil {
		return err
	}
	if _, err := m.takeSnapshot(r, "before rollback to "+displayVersion(sn.Version)); err != nil {
		return err
	}
	var current []string
	if s.Kind == KindNPXInstaller {
		var err error
		if current, err = resolveFootprint(footprintPatterns(s, m.opts.UserHome, m.scopeDir(s)), m.opts.UserHome, m.scopeDir(s)); err != nil {
			return err
		}
	}
	r.log("Restoring %s …", src)
	if err := restoreSnapshot(src, m.opts.UserHome, current); err != nil {
		return err
	}
	if s.Kind == KindClaudePlugin {
		if err := m.restorePluginEntries(r, sn); err != nil {
			return err
		}
	}
	m.pruneSnapshots(r)
	d := m.detect(s)
	_ = m.store.update(s.ID, func(p *PackageState) { p.Active = d.version })
	r.message = fmt.Sprintf("%s restored to %s from the snapshot of %s.", s.Name, displayVersion(sn.Version), sn.CreatedAt.Format(time.RFC3339))
	if s.Kind == KindClaudePlugin {
		r.message += " Restart Claude Code to load it."
	}
	return nil
}

// uninstallInstaller snapshots the package, runs its own uninstaller when the manifest names one,
// and removes whatever of its footprint is left.
func (m *Manager) uninstallInstaller(r *jobRun) error {
	s := r.spec
	if _, err := m.takeSnapshot(r, "before uninstall"); err != nil {
		return err
	}
	m.pruneSnapshots(r)
	if len(s.UninstArgs) > 0 {
		v := m.detectInstaller(s).version
		if v == "" || v == VersionUnknown {
			v = "latest"
		}
		argv := append([]string{"npx", "-y", s.Package + "@" + v}, m.installerArgs(s, s.UninstArgs, v)...)
		if err := r.run(argv, nil, m.scopeDir(s)); err != nil {
			r.log("The installer's own uninstall failed (%v); removing the files directly.", err)
		}
	}
	rels, err := resolveFootprint(footprintPatterns(s, m.opts.UserHome, m.scopeDir(s)), m.opts.UserHome, m.scopeDir(s))
	if err != nil {
		return err
	}
	for _, rel := range rels {
		r.log("Removing ~/%s", filepath.ToSlash(rel))
	}
	if err := removeFootprint(m.opts.UserHome, rels); err != nil {
		return err
	}
	_ = m.store.update(s.ID, func(p *PackageState) { p.Active = "" })
	r.message = s.Name + " is uninstalled; a snapshot was kept, so a rollback can restore it."
	return nil
}
