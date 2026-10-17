// Package packages installs, updates, pins, rolls back and uninstalls the tools the orchestrator
// itself relies on (configs/packages.json): the Claude Code CLI, methodologies such as BMAD and
// GSD, Claude Code plugins, browsers for evidence, runtimes and MCP server binaries.
//
// Each manifest entry has a kind that decides how it is managed:
//
//   - npm-cli and go-cli packages get versioned installs under <home>/packages/<id>/<version> and
//     a shim in <home>/bin that is switched atomically, so a rollback is instant and offline.
//   - npx-installer packages (BMAD, GSD) write into the operator's home; their footprint is
//     snapshotted before every change, and a rollback restores a snapshot.
//   - claude-plugin packages go through `claude plugin …`; their cache folder and entry in
//     installed_plugins.json are snapshotted before an update or uninstall.
//   - system packages (Node, Git, Docker, ffmpeg) go through Homebrew or the toolchain catalog;
//     system package managers keep no previous versions, so they cannot be rolled back.
//
// No command ever runs through a shell; every command is an argv list built from the manifest and
// a validated version.
package packages

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Kinds of package.
const (
	KindNPMCLI       = "npm-cli"
	KindGoCLI        = "go-cli"
	KindNPXInstaller = "npx-installer"
	KindClaudePlugin = "claude-plugin"
	KindSystem       = "system"
)

// Categories a manifest entry may use.
var Categories = []string{"Agents", "Methodologies", "Plugins", "Runtimes", "Media", "Connectors"}

// VersionFile locates the installed version of an npx-installer package.
type VersionFile struct {
	Path  string `json:"path"`  // "~/_bmad/_config/manifest.yaml"; ~ and {dir} are expanded
	Regex string `json:"regex"` // the first capture group is the version
}

// SystemSource names the package in system package managers.
type SystemSource struct {
	Brew string `json:"brew,omitempty"` // Homebrew formula
	Apt  string `json:"apt,omitempty"`  // Debian/Ubuntu package; shown as a command, never run (sudo)
}

// Spec is one configs/packages.json entry.
type Spec struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Category    string       `json:"category"`
	Description string       `json:"description"`
	Docs        string       `json:"docs"`
	Kind        string       `json:"kind"`
	Package     string       `json:"package"`        // npm package, go install path, plugin id
	Module      string       `json:"module"`         // go-cli: module path whose versions are listed
	Binaries    []string     `json:"binaries"`       // commands the package provides; the first is version-checked
	VersionArgs []string     `json:"version_args"`   // default ["--version"]
	Required    bool         `json:"required"`       // the orchestrator cannot run its pipeline without it
	Requires    []string     `json:"requires"`       // package ids to install first
	ScopeDir    string       `json:"scope_dir"`      // npx-installer: {dir}; default "~"
	InstallArgs []string     `json:"install_args"`   // npx-installer: arguments after <package>@<version>
	UpdateArgs  []string     `json:"update_args"`    // npx-installer: arguments when already installed
	UninstArgs  []string     `json:"uninstall_args"` // npx-installer: the installer's own uninstaller, run before the footprint is removed
	VersionFile *VersionFile `json:"version_file,omitempty"`
	Footprint   []string     `json:"footprint"` // npx-installer: globs (~ and {dir} expanded) snapshotted before changes
	// FootprintFrom adds the entries an installer records in its own manifest (BMAD lists every
	// skill it installs in skill-manifest.csv), so skills with other prefixes are covered too.
	FootprintFrom *FootprintList `json:"footprint_from,omitempty"`
	PostInstall   [][]string     `json:"post_install"` // argv lists run after an install; argv[0] may name one of Binaries
	ToolchainID   string         `json:"toolchain_id"` // system: the internal/toolchain catalog entry
	System        SystemSource   `json:"system"`
	Notes         string         `json:"notes"`
}

// FootprintList reads extra footprint entries from one column of an installer's CSV manifest:
// each value is a single file or folder name joined to Prefix.
type FootprintList struct {
	CSV    string `json:"csv"`    // ~ or {dir} path of the CSV file
	Column string `json:"column"` // header of the column holding the names
	Prefix string `json:"prefix"` // ~ or {dir} folder the names live in
}

// Manifest is the parsed configs/packages.json.
type Manifest struct {
	Packages []*Spec
	byID     map[string]*Spec
}

var (
	idRe      = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	versionRe = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,63}$`)
	binaryRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	// npm package names and go install paths, and plugin ids "name@marketplace".
	packageRe = regexp.MustCompile(`^[@A-Za-z0-9][A-Za-z0-9@/._-]{0,213}$`)
	aptRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9.+-]*( [a-z0-9][a-z0-9.+-]*)*$`)
)

// Errors about input.
var (
	ErrInvalidVersion = errors.New("invalid version: use letters, digits, '.', '-', '+' or '_' (at most 64)")
	ErrInvalidID      = errors.New("invalid package id")
)

// ValidateVersion accepts "" (meaning latest) or a plain version label.
func ValidateVersion(v string) error {
	if v == "" {
		return nil
	}
	if !versionRe.MatchString(v) || strings.Contains(v, "..") {
		return ErrInvalidVersion
	}
	return nil
}

// ValidID reports whether id is a well-formed package id.
func ValidID(id string) bool { return idRe.MatchString(id) }

// LoadManifest reads and validates a manifest file.
func LoadManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseManifest(data)
}

// ParseManifest validates a manifest: unknown kinds, unsafe names and dangling requirements are
// errors, so a typo never turns into a command.
func ParseManifest(data []byte) (*Manifest, error) {
	var specs []*Spec
	if err := json.Unmarshal(data, &specs); err != nil {
		return nil, fmt.Errorf("packages manifest: %w", err)
	}
	m := &Manifest{byID: map[string]*Spec{}}
	for i, s := range specs {
		if err := s.validate(); err != nil {
			return nil, fmt.Errorf("packages manifest entry %d (%s): %w", i, s.ID, err)
		}
		if m.byID[s.ID] != nil {
			return nil, fmt.Errorf("packages manifest: duplicate id %q", s.ID)
		}
		m.byID[s.ID] = s
		m.Packages = append(m.Packages, s)
	}
	for _, s := range m.Packages {
		for _, r := range s.Requires {
			if m.byID[r] == nil {
				return nil, fmt.Errorf("packages manifest: %s requires unknown package %q", s.ID, r)
			}
		}
	}
	if _, err := m.order(idsOf(m.Packages)); err != nil {
		return nil, err
	}
	return m, nil
}

func idsOf(specs []*Spec) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = s.ID
	}
	return out
}

// Get returns the entry with id.
func (m *Manifest) Get(id string) *Spec {
	if m == nil {
		return nil
	}
	return m.byID[id]
}

// order returns ids and everything they require, dependencies first, in manifest order otherwise.
func (m *Manifest) order(ids []string) ([]string, error) {
	var out []string
	state := map[string]int{} // 1 visiting, 2 done
	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("packages manifest: requirement cycle %s", strings.Join(append(path, id), " → "))
		case 2:
			return nil
		}
		state[id] = 1
		for _, r := range m.byID[id].Requires {
			if err := visit(r, append(path, id)); err != nil {
				return err
			}
		}
		state[id] = 2
		out = append(out, id)
		return nil
	}
	for _, id := range ids {
		if m.byID[id] == nil {
			return nil, fmt.Errorf("unknown package %q", id)
		}
		if err := visit(id, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Spec) validate() error {
	if !idRe.MatchString(s.ID) {
		return ErrInvalidID
	}
	if s.Name == "" {
		return errors.New("name is required")
	}
	okCat := false
	for _, c := range Categories {
		okCat = okCat || c == s.Category
	}
	if !okCat {
		return fmt.Errorf("category %q is not one of %s", s.Category, strings.Join(Categories, ", "))
	}
	for _, b := range s.Binaries {
		if !binaryRe.MatchString(b) {
			return fmt.Errorf("invalid binary name %q", b)
		}
	}
	if s.Package != "" && (!packageRe.MatchString(s.Package) || strings.Contains(s.Package, "..")) {
		return fmt.Errorf("invalid package %q", s.Package)
	}
	if s.Module != "" && (!packageRe.MatchString(s.Module) || strings.Contains(s.Module, "..")) {
		return fmt.Errorf("invalid module %q", s.Module)
	}
	if s.System.Brew != "" && !packageRe.MatchString(s.System.Brew) {
		return fmt.Errorf("invalid brew formula %q", s.System.Brew)
	}
	if s.System.Apt != "" && !aptRe.MatchString(s.System.Apt) {
		return fmt.Errorf("invalid apt packages %q", s.System.Apt)
	}
	switch s.Kind {
	case KindNPMCLI, KindGoCLI:
		if s.Package == "" || len(s.Binaries) == 0 {
			return errors.New("package and binaries are required")
		}
		if s.Kind == KindGoCLI && s.Module == "" {
			return errors.New("module is required for go-cli")
		}
	case KindNPXInstaller:
		if s.ScopeDir != "" && s.ScopeDir != "~" && checkFootprintPattern(s.ScopeDir) != nil {
			return fmt.Errorf("scope_dir %q must be ~ or a folder below it", s.ScopeDir)
		}
		if strings.Contains(s.ScopeDir, "{dir}") {
			return errors.New("scope_dir cannot use {dir}")
		}
		if s.Package == "" || len(s.Footprint) == 0 || s.VersionFile == nil {
			return errors.New("package, footprint and version_file are required")
		}
		if _, err := regexp.Compile(s.VersionFile.Regex); err != nil {
			return fmt.Errorf("version_file.regex: %w", err)
		}
		for _, f := range s.Footprint {
			if err := checkFootprintPattern(f); err != nil {
				return err
			}
		}
		if err := checkFootprintPattern(s.VersionFile.Path); err != nil {
			return fmt.Errorf("version_file.path: %w", err)
		}
		if f := s.FootprintFrom; f != nil {
			if f.Column == "" {
				return errors.New("footprint_from.column is required")
			}
			if err := checkFootprintPattern(f.CSV); err != nil {
				return fmt.Errorf("footprint_from.csv: %w", err)
			}
			if err := checkFootprintPattern(strings.TrimSuffix(f.Prefix, "/")); err != nil {
				return fmt.Errorf("footprint_from.prefix: %w", err)
			}
		}
	case KindClaudePlugin:
		if s.Package == "" || !strings.Contains(s.Package, "@") {
			return errors.New("package must be <plugin>@<marketplace>")
		}
	case KindSystem:
		if len(s.Binaries) == 0 {
			return errors.New("binaries are required")
		}
	default:
		return fmt.Errorf("unknown kind %q", s.Kind)
	}
	for _, argv := range s.PostInstall {
		if len(argv) == 0 {
			return errors.New("empty post_install command")
		}
	}
	return nil
}

// checkFootprintPattern accepts only patterns that stay inside the home folder and name a fixed
// first folder below it, so a pattern can never match the home folder itself or all of it.
func checkFootprintPattern(p string) error {
	rest, ok := strings.CutPrefix(p, "~/")
	if !ok {
		rest, ok = strings.CutPrefix(p, "{dir}/")
	}
	if !ok {
		return fmt.Errorf("footprint %q must start with ~/ or {dir}/", p)
	}
	parts := strings.Split(rest, "/")
	if rest == "" || strings.ContainsAny(parts[0], "*?[") {
		return fmt.Errorf("footprint %q must name a fixed folder below the home folder", p)
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("footprint %q is not a clean relative path", p)
		}
	}
	return nil
}

// expand replaces ~ and {dir} in a manifest path or argument.
func expand(s, home, dir string) string {
	if s == "~" {
		return home
	}
	if rest, ok := strings.CutPrefix(s, "~/"); ok {
		s = filepath.Join(home, rest)
	}
	s = strings.ReplaceAll(s, "{dir}", dir)
	return s
}
