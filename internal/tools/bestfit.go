package tools

import (
	"fmt"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

// RuntimeCheck is one required runtime of a tool, as found on this machine.
type RuntimeCheck struct {
	Runtime    string `json:"runtime"`
	Constraint string `json:"constraint"`
	Found      string `json:"found,omitempty"` // detected version
	Satisfied  bool   `json:"satisfied"`
	Problem    string `json:"problem,omitempty"`
}

// Compatibility is whether a tool can run on this machine.
type Compatibility struct {
	ToolID           string         `json:"tool_id"`
	Name             string         `json:"name"`
	Status           string         `json:"status"`
	InstalledVersion string         `json:"installed_version,omitempty"`
	Runtimes         []RuntimeCheck `json:"runtimes"`
	Compatible       bool           `json:"compatible"`
}

// Matrix is the compatibility of every manifest tool with this host.
type Matrix struct {
	HostOS    string          `json:"host_os"`
	HostArch  string          `json:"host_arch"`
	Status    string          `json:"matrix_status"` // "compatible" or "conflicts"
	Tools     []Compatibility `json:"tools"`
	Conflicts []string        `json:"conflicts"`
}

// runtimeCatalog maps a required_runtimes key to the toolchain catalog entry that detects it.
var runtimeCatalog = map[string]string{"node": "node", "docker": "docker", "go": "go", "java": "java"}

// ResolveMatrix checks every tool's required runtimes against the versions detected here.
func (tm *ToolManager) ResolveMatrix() Matrix {
	m := Matrix{HostOS: runtime.GOOS, HostArch: runtime.GOARCH, Status: "compatible", Tools: []Compatibility{}, Conflicts: []string{}}
	for _, ts := range tm.List() {
		c := Compatibility{ToolID: ts.ID, Name: ts.Name, Status: ts.Status, InstalledVersion: ts.InstalledVersion,
			Runtimes: []RuntimeCheck{}, Compatible: true}
		for _, name := range sortedKeys(ts.RequiredRuntimes) {
			rc := tm.checkRuntime(name, ts.RequiredRuntimes[name])
			if !rc.Satisfied {
				c.Compatible = false
				m.Conflicts = append(m.Conflicts, fmt.Sprintf("%s needs %s %s: %s", ts.Name, name, rc.Constraint, rc.Problem))
			}
			c.Runtimes = append(c.Runtimes, rc)
		}
		m.Tools = append(m.Tools, c)
	}
	if len(m.Conflicts) > 0 {
		m.Status = "conflicts"
	}
	return m
}

func (tm *ToolManager) checkRuntime(name, constraint string) RuntimeCheck {
	rc := RuntimeCheck{Runtime: name, Constraint: constraint}
	id, ok := runtimeCatalog[name]
	t, known := toolchain.Lookup(id)
	if !ok || !known {
		rc.Problem = "the orchestrator cannot detect this runtime"
		return rc
	}
	s := tm.tc.Checker.Check(t)
	if !s.Installed {
		rc.Problem = "not installed"
		if s.Problem != "" {
			rc.Problem = s.Problem
		}
		return rc
	}
	rc.Found = ParseVersion(s.Version)
	min, ok := strings.CutPrefix(strings.TrimSpace(constraint), ">=")
	if !ok {
		rc.Problem = "unsupported constraint (only >=X.Y.Z)"
		return rc
	}
	if rc.Found == "" {
		rc.Problem = "could not read its version from " + s.Version
		return rc
	}
	rc.Satisfied = compareVersions(rc.Found, strings.TrimSpace(min)) >= 0
	if !rc.Satisfied {
		rc.Problem = "found " + rc.Found
	}
	return rc
}

// compareVersions compares dotted numeric versions; missing parts count as zero.
func compareVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
