package packages

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// npm-cli and go-cli packages are installed per version under <home>/packages/<id>/<version>;
// <home>/bin/<binary> is a symbolic link to the active version's binary, replaced atomically.

func (m *Manager) versionDir(id, version string) string {
	return filepath.Join(m.packageDir(id), "versions", version)
}

// binaryPath is where an install of version keeps binary.
func (m *Manager) binaryPath(s *Spec, version, binary string) string {
	dir := m.versionDir(s.ID, version)
	if s.Kind == KindGoCLI {
		return filepath.Join(dir, "bin", binary)
	}
	return filepath.Join(dir, "node_modules", ".bin", binary)
}

func (m *Manager) binariesPresent(s *Spec, version string) bool {
	for _, b := range s.Binaries {
		if _, err := os.Stat(m.binaryPath(s, version, b)); err != nil {
			return false
		}
	}
	return true
}

// shimOwned reports whether shim is a link into this package's managed versions.
func (m *Manager) shimOwned(s *Spec, shim string) bool {
	target, err := os.Readlink(shim)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(shim), target)
	}
	_, err = relInside(filepath.Join(m.packageDir(s.ID), "versions"), target)
	return err == nil
}

// switchShim points <bin>/<name> at target atomically: a new link is created beside it and renamed
// over the old one, so the command is never missing or half-switched.
func switchShim(binDir, name, target string) error {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(binDir, fmt.Sprintf(".%s.tmp-%d-%d", name, os.Getpid(), time.Now().UnixNano()))
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, filepath.Join(binDir, name)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func goVersion(v string) string {
	if v != "" && v[0] >= '0' && v[0] <= '9' {
		return "v" + v
	}
	return v
}

func (m *Manager) needTool(name, why string) error {
	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s is not on PATH: %s", name, why)
	}
	return nil
}

func (m *Manager) managerTool(s *Spec) (string, string) {
	if s.Kind == KindGoCLI {
		return "go", "install Go first (https://go.dev/doc/install)"
	}
	return "npm", "install Node.js first (the node package)"
}

// installManaged installs version ("" = latest) and makes it active. A version that is already
// kept is only switched to.
func (m *Manager) installManaged(r *jobRun, version string) error {
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
	if ValidateVersion(version) != nil {
		return ErrInvalidVersion
	}
	st := m.store.get(s.ID)
	if st.kept(version) != nil && m.binariesPresent(s, version) {
		r.log("Version %s is already installed under %s; switching to it.", version, m.versionDir(s.ID, version))
	} else {
		dir := m.versionDir(s.ID, version)
		if err := os.RemoveAll(dir); err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		var err error
		if s.Kind == KindGoCLI {
			err = r.run([]string{"go", "install", s.Package + "@" + goVersion(version)},
				[]string{"GOBIN=" + filepath.Join(dir, "bin")}, dir)
		} else {
			err = r.run([]string{"npm", "install", "--prefix", dir, "--no-fund", "--no-audit", s.Package + "@" + version}, nil, dir)
		}
		if err == nil && !m.binariesPresent(s, version) {
			err = fmt.Errorf("the install finished but %s is missing", strings.Join(s.Binaries, ", "))
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
		now := time.Now().UTC()
		if err := m.store.update(s.ID, func(p *PackageState) {
			if k := p.kept(version); k != nil {
				k.InstalledAt = now
			} else {
				p.Versions = append(p.Versions, KeptVersion{Version: version, InstalledAt: now})
			}
		}); err != nil {
			return err
		}
	}
	if err := m.activate(r, version); err != nil {
		return err
	}
	for _, argv := range s.PostInstall {
		cmd := append([]string(nil), argv...)
		for _, b := range s.Binaries {
			if cmd[0] == b {
				cmd[0] = filepath.Join(m.BinDir(), b)
			}
		}
		if err := r.run(cmd, nil, m.opts.Home); err != nil {
			return fmt.Errorf("post-install step: %w", err)
		}
	}
	m.prune(r)
	return nil
}

// activate switches every shim to version and records it as active.
func (m *Manager) activate(r *jobRun, version string) error {
	s := r.spec
	for _, b := range s.Binaries {
		if err := switchShim(m.BinDir(), b, m.binaryPath(s, version, b)); err != nil {
			return fmt.Errorf("switching %s: %w", b, err)
		}
		r.log("%s → %s", filepath.Join(m.BinDir(), b), m.binaryPath(s, version, b))
	}
	now := time.Now().UTC()
	return m.store.update(s.ID, func(p *PackageState) {
		if old := p.kept(p.Active); old != nil && p.Active != version {
			old.LastActiveAt = now
		}
		p.Active = version
		if k := p.kept(version); k != nil {
			k.LastActiveAt = now
		}
	})
}

// prune removes inactive versions beyond the newest keepInactive.
func (m *Manager) prune(r *jobRun) {
	var dropped []string
	_ = m.store.update(r.spec.ID, func(p *PackageState) { dropped = p.pruneVersions() })
	for _, v := range dropped {
		if err := os.RemoveAll(m.versionDir(r.spec.ID, v)); err != nil {
			r.log("Could not remove old version %s: %v", v, err)
		} else {
			r.log("Removed old version %s.", v)
		}
	}
}

// rollbackManaged switches to a kept version, or installs it.
func (m *Manager) rollbackManaged(r *jobRun, version string) error {
	st := m.store.get(r.spec.ID)
	if st.kept(version) != nil && m.binariesPresent(r.spec, version) {
		if err := m.activate(r, version); err != nil {
			return err
		}
		r.message = fmt.Sprintf("%s rolled back to %s (no download needed).", r.spec.Name, version)
		return nil
	}
	r.log("Version %s is not kept here; installing it.", version)
	return m.installManaged(r, version)
}

// uninstallManaged removes the shims and every managed version; a system copy is left alone.
func (m *Manager) uninstallManaged(r *jobRun) error {
	s := r.spec
	for _, b := range s.Binaries {
		shim := filepath.Join(m.BinDir(), b)
		if m.shimOwned(s, shim) {
			if err := os.Remove(shim); err != nil {
				return err
			}
			r.log("Removed %s", shim)
		}
	}
	dir := filepath.Join(m.packageDir(s.ID), "versions")
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	r.log("Removed %s", dir)
	if err := m.store.update(s.ID, func(p *PackageState) { p.Active, p.Versions = "", nil }); err != nil {
		return err
	}
	if sys := lookPathExcept(s.Binaries[0], m.BinDir()); sys != "" {
		r.message = fmt.Sprintf("%s's managed versions are removed; the system copy at %s is still on PATH.", s.Name, sys)
	}
	return nil
}
