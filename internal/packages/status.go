package packages

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

// Package states.
const (
	StatusInstalled    = "installed"
	StatusNotInstalled = "not_installed"
	StatusError        = "error"
	StatusChecking     = "checking"
)

// VersionUnknown is reported when a package is present but its version cannot be read.
const VersionUnknown = "unknown"

// InstalledVersion is a kept install or a snapshot.
type InstalledVersion struct {
	Version     string `json:"version"`
	InstalledAt string `json:"installed_at"`
	Active      bool   `json:"active"`
	Kind        string `json:"kind"` // "install" or "snapshot"
}

// Actions says which actions the package offers here.
type Actions struct {
	Install     bool `json:"install"`
	Update      bool `json:"update"`
	Rollback    bool `json:"rollback"`
	Uninstall   bool `json:"uninstall"`
	PickVersion bool `json:"pick_version"`
}

// ActionNotes explain an action, or why it is unavailable.
type ActionNotes struct {
	Install   string `json:"install,omitempty"`
	Update    string `json:"update,omitempty"`
	Rollback  string `json:"rollback,omitempty"`
	Uninstall string `json:"uninstall,omitempty"`
}

// Package is a manifest entry with what this machine has of it.
type Package struct {
	ID                string             `json:"id"`
	Name              string             `json:"name"`
	Category          string             `json:"category"`
	Description       string             `json:"description"`
	Docs              string             `json:"docs"`
	Kind              string             `json:"kind"`
	Package           string             `json:"package"`
	Required          bool               `json:"required"`
	Requires          []string           `json:"requires"`
	Status            string             `json:"status"`
	Version           string             `json:"version"`
	Latest            string             `json:"latest"`
	UpdateAvailable   bool               `json:"update_available"`
	Managed           bool               `json:"managed"`
	Location          string             `json:"location"`
	SystemCopy        string             `json:"system_copy"`
	InstalledVersions []InstalledVersion `json:"installed_versions"`
	Actions           Actions            `json:"actions"`
	ActionNotes       ActionNotes        `json:"action_notes"`
	Manual            *Manual            `json:"manual"`
	Job               *Job               `json:"job"`
	History           []HistoryEntry     `json:"history"`
	Notes             string             `json:"notes"`
	// Problem explains status "error" (not part of the original contract; additive).
	Problem string `json:"problem,omitempty"`
}

// Listing is GET /api/v1/packages.
type Listing struct {
	Home     string    `json:"home"`
	BinDir   string    `json:"bin_dir"`
	OnPath   bool      `json:"on_path"`
	Packages []Package `json:"packages"`
}

// detection is what is installed of a package.
type detection struct {
	status     string
	version    string
	managed    bool
	location   string
	systemCopy string
	problem    string
}

// List reports every package. Version checks run in parallel and are cached for a minute; the
// latest versions come from a cache refreshed in the background.
func (m *Manager) List() Listing {
	out := Listing{Home: m.opts.Home, BinDir: m.BinDir(), OnPath: m.OnPath(), Packages: make([]Package, len(m.manifest.Packages))}
	var wg sync.WaitGroup
	for i, s := range m.manifest.Packages {
		wg.Add(1)
		go func(i int, s *Spec) {
			defer wg.Done()
			out.Packages[i] = m.view(s)
		}(i, s)
	}
	wg.Wait()
	return out
}

// Get reports one package.
func (m *Manager) Get(id string) (Package, error) {
	s, err := m.spec(id)
	if err != nil {
		return Package{}, err
	}
	return m.view(s), nil
}

// Recheck forgets cached checks and reports every package again.
func (m *Manager) Recheck() Listing {
	toolchain.EnsurePath()
	m.Invalidate()
	return m.List()
}

func (m *Manager) view(s *Spec) Package {
	d := m.detect(s)
	st := m.store.get(s.ID)
	p := Package{ID: s.ID, Name: s.Name, Category: s.Category, Description: s.Description, Docs: s.Docs, Kind: s.Kind,
		Package: s.Package, Required: s.Required, Requires: append([]string{}, s.Requires...),
		Status: d.status, Version: d.version, Managed: d.managed, Location: d.location, SystemCopy: d.systemCopy,
		Problem: d.problem, Notes: s.Notes, InstalledVersions: []InstalledVersion{}, History: []HistoryEntry{},
		Job: m.LatestJob(s.ID)}
	p.Latest = m.latestVersion(s)
	if d.status == StatusInstalled && p.Latest != "" && d.version != "" && d.version != VersionUnknown {
		p.UpdateAvailable = compareVersions(p.Latest, d.version) > 0
	}
	for i := len(st.History) - 1; i >= 0 && len(p.History) < 20; i-- {
		p.History = append(p.History, st.History[i])
	}
	p.InstalledVersions = m.installedVersions(s, st, d)
	p.Actions, p.ActionNotes, p.Manual = m.actions(s, st, d)
	return p
}

func (m *Manager) installedVersions(s *Spec, st PackageState, d detection) []InstalledVersion {
	out := []InstalledVersion{}
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		vs := append([]KeptVersion(nil), st.Versions...)
		sort.SliceStable(vs, func(i, j int) bool { return compareVersions(vs[i].Version, vs[j].Version) > 0 })
		for _, v := range vs {
			out = append(out, InstalledVersion{Version: v.Version, InstalledAt: stamp(v.InstalledAt),
				Active: d.managed && v.Version == st.Active, Kind: "install"})
		}
	default:
		if d.status == StatusInstalled {
			out = append(out, InstalledVersion{Version: d.version, InstalledAt: m.installedAt(s, d), Active: true, Kind: "install"})
		}
		for i := len(st.Snapshots) - 1; i >= 0; i-- {
			sn := st.Snapshots[i]
			v := sn.Version
			if v == "" {
				v = VersionUnknown
			}
			out = append(out, InstalledVersion{Version: v, InstalledAt: stamp(sn.CreatedAt), Kind: "snapshot"})
		}
	}
	return out
}

// installedAt is the modification time of what marks the install, when known.
func (m *Manager) installedAt(s *Spec, d detection) string {
	path := ""
	switch s.Kind {
	case KindNPXInstaller:
		path = m.versionFilePath(s)
	case KindClaudePlugin:
		path = d.location
	}
	if path == "" {
		return ""
	}
	if st, err := os.Stat(path); err == nil {
		return stamp(st.ModTime())
	}
	return ""
}

// detect reports what this machine has of s.
func (m *Manager) detect(s *Spec) detection {
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		return m.detectManaged(s)
	case KindNPXInstaller:
		return m.detectInstaller(s)
	case KindClaudePlugin:
		return m.detectPlugin(s)
	}
	return m.detectSystem(s)
}

func versionArgs(s *Spec) []string {
	if len(s.VersionArgs) > 0 {
		return s.VersionArgs
	}
	return []string{"--version"}
}

// checkBinary runs path's version command (cached for a minute, bounded by checkTimeout).
func (m *Manager) checkBinary(cacheID, path string, s *Spec) toolchain.Status {
	return m.tc.Checker.Check(toolchain.Tool{ID: cacheID, Name: s.Name, Binary: path, VersionArgs: versionArgs(s), Timeout: checkTimeout})
}

func (m *Manager) detectManaged(s *Spec) detection {
	d := detection{status: StatusNotInstalled}
	st := m.store.get(s.ID)
	bin := s.Binaries[0]
	shim := filepath.Join(m.BinDir(), bin)
	if st.Active != "" && m.shimOwned(s, shim) {
		d.managed, d.location = true, shim
		c := m.checkBinary("pkg:"+s.ID+":managed", shim, s)
		switch {
		case c.Installed:
			d.status, d.version = StatusInstalled, parseVersion(c.Version)
			if d.version == "" {
				d.version = st.Active
			}
		default:
			d.status, d.version, d.problem = StatusError, st.Active, c.Problem
			if d.problem == "" {
				d.problem = "the managed shim " + shim + " does not run"
			}
		}
	}
	if sys := lookPathExcept(bin, m.BinDir()); sys != "" {
		d.systemCopy = sys
		if !d.managed {
			c := m.checkBinary("pkg:"+s.ID+":system", sys, s)
			d.location = sys
			switch {
			case c.Installed:
				d.status, d.version = StatusInstalled, parseVersion(c.Version)
				if d.version == "" {
					d.version = VersionUnknown
				}
			default:
				d.status, d.problem = StatusError, c.Problem
			}
		}
	}
	return d
}

func (m *Manager) scopeDir(s *Spec) string {
	if s.ScopeDir == "" {
		return m.opts.UserHome
	}
	return expand(s.ScopeDir, m.opts.UserHome, "")
}

func (m *Manager) versionFilePath(s *Spec) string {
	if s.VersionFile == nil {
		return ""
	}
	return expand(s.VersionFile.Path, m.opts.UserHome, m.scopeDir(s))
}

func (m *Manager) detectInstaller(s *Spec) detection {
	d := detection{status: StatusNotInstalled}
	path := m.versionFilePath(s)
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			d.status, d.problem = StatusError, "cannot read "+path+": "+err.Error()
		}
		return d
	}
	d.status, d.location, d.managed = StatusInstalled, path, true
	re := regexp.MustCompile(s.VersionFile.Regex)
	if mm := re.FindSubmatch(raw); len(mm) > 1 && ValidateVersion(strings.TrimSpace(string(mm[1]))) == nil {
		d.version = strings.TrimSpace(string(mm[1]))
	} else {
		d.version = VersionUnknown
	}
	return d
}

func (m *Manager) detectSystem(s *Spec) detection {
	d := detection{status: StatusNotInstalled}
	t := toolchain.Tool{ID: "pkg:" + s.ID, Name: s.Name, Binary: s.Binaries[0], VersionArgs: versionArgs(s), Timeout: checkTimeout}
	if s.ToolchainID != "" {
		if ct, ok := toolchain.Lookup(s.ToolchainID); ok && len(s.VersionArgs) == 0 {
			t.Binary, t.VersionArgs = ct.Binary, ct.VersionArgs
		}
	}
	c := m.tc.Checker.Check(t)
	d.location = c.Path
	switch {
	case c.Installed:
		d.status, d.version = StatusInstalled, parseVersion(c.Version)
		if d.version == "" {
			d.version = VersionUnknown
		}
	case c.Problem != "":
		d.status, d.problem = StatusError, c.Problem
	}
	return d
}
