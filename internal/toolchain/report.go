package toolchain

import (
	"context"
	"sort"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
)

// Requirement is a tool the scope (a task, a project, every project) needs, and why.
type Requirement struct {
	Tool     string   `json:"tool"`
	Required bool     `json:"required"`
	Reasons  []string `json:"reasons"` // "billing: build and test the Go code"
}

// Requirements merges the needs of several repositories: a tool is required when any of them
// requires it.
func Requirements(repos []stack.Repo) []Requirement {
	var out []Requirement
	index := map[string]int{}
	for _, r := range repos {
		for _, n := range r.Profile.Needs() {
			reason := r.Name + ": " + n.Why
			if i, ok := index[n.Tool]; ok {
				out[i].Required = out[i].Required || n.Required
				out[i].Reasons = append(out[i].Reasons, reason)
				continue
			}
			index[n.Tool] = len(out)
			out = append(out, Requirement{Tool: n.Tool, Required: n.Required, Reasons: []string{reason}})
		}
	}
	return out
}

// Merge combines requirement lists: a tool is required when any list requires it, and keeps every
// reason.
func Merge(lists ...[]Requirement) []Requirement {
	var out []Requirement
	index := map[string]int{}
	for _, list := range lists {
		for _, r := range list {
			if i, ok := index[r.Tool]; ok {
				out[i].Required = out[i].Required || r.Required
				out[i].Reasons = append(out[i].Reasons, r.Reasons...)
				continue
			}
			index[r.Tool] = len(out)
			r.Reasons = append([]string(nil), r.Reasons...)
			out = append(out, r)
		}
	}
	return out
}

// MethodView is an install method as this machine sees it.
type MethodView struct {
	Index     int    `json:"index"`
	Manager   string `json:"manager"`
	Command   string `json:"command"`
	Auto      bool   `json:"auto"`
	Available bool   `json:"available"`
	// Unavailable says why the method cannot run here (e.g. Homebrew is missing).
	Unavailable string `json:"unavailable,omitempty"`
	Note        string `json:"note,omitempty"`
}

// Item is one tool in a report.
type Item struct {
	Tool
	Status
	Needed   bool         `json:"needed"`   // the scope's repositories need it
	Required bool         `json:"required"` // and cannot be verified without it
	Reasons  []string     `json:"reasons,omitempty"`
	Methods  []MethodView `json:"methods"`
	// Recommended is the MethodView.Index of the method to offer first, or -1 when only the docs
	// can help on this machine.
	Recommended int  `json:"recommended"`
	Job         *Job `json:"job,omitempty"`
}

// Report is the toolchain readiness of a scope.
type Report struct {
	Platform string `json:"platform"`
	Items    []Item `json:"items"`
	// Missing are the required tools that are not usable; MissingRecommended the others.
	Missing            []string `json:"missing"`
	MissingRecommended []string `json:"missing_recommended"`
	Ready              bool     `json:"ready"` // nothing required is missing
}

// Toolchain checks and installs tools for the orchestrator.
type Toolchain struct {
	Checker   *Checker
	Installer *Installer
}

// New checks and installs tools on this machine.
func New() *Toolchain {
	c := NewChecker()
	return &Toolchain{Checker: c, Installer: NewInstaller(c)}
}

// Report checks the required tools (and, with all, every catalog tool) and how to install the
// missing ones here. Needed tools come first: required, then recommended, each in catalog order.
func (tc *Toolchain) Report(reqs []Requirement, all bool) Report {
	byTool := map[string]Requirement{}
	for _, r := range reqs {
		byTool[r.Tool] = r
	}
	var tools []Tool
	for _, t := range Catalog {
		if _, needed := byTool[t.ID]; needed || all {
			tools = append(tools, t)
		}
	}
	statuses := tc.Checker.CheckAll(tools)
	rep := Report{Platform: tc.Checker.host.goos, Missing: []string{}, MissingRecommended: []string{}}
	for _, t := range tools {
		req, needed := byTool[t.ID]
		it := Item{Tool: t, Status: statuses[t.ID], Needed: needed, Required: req.Required, Reasons: req.Reasons, Recommended: -1}
		for i, m := range t.Methods {
			ok, why := tc.Checker.managerAvailable(m)
			if why == "not for "+tc.Checker.host.goos {
				continue // never offer a macOS command on Linux and vice versa
			}
			it.Methods = append(it.Methods, MethodView{Index: i, Manager: m.Manager, Command: strings.Join(m.Command, " "),
				Auto: m.Auto, Available: ok, Unavailable: why, Note: m.Note})
		}
		it.Recommended = recommend(it.Methods)
		if tc.Installer != nil {
			it.Job = tc.Installer.Latest(t.ID)
		}
		if needed && !it.Installed {
			if it.Required {
				rep.Missing = append(rep.Missing, t.ID)
			} else {
				rep.MissingRecommended = append(rep.MissingRecommended, t.ID)
			}
		}
		rep.Items = append(rep.Items, it)
	}
	sort.SliceStable(rep.Items, func(i, j int) bool { return rank(rep.Items[i]) < rank(rep.Items[j]) })
	rep.Ready = len(rep.Missing) == 0
	return rep
}

func rank(it Item) int {
	switch {
	case it.Needed && it.Required:
		return 0
	case it.Needed:
		return 1
	}
	return 2
}

// recommend prefers a one-click method that can run here, then one the operator can run here.
func recommend(methods []MethodView) int {
	for _, m := range methods {
		if m.Auto && m.Available {
			return m.Index
		}
	}
	for _, m := range methods {
		if m.Available {
			return m.Index
		}
	}
	return -1
}

// Names returns "OpenTofu (tofu), Go (go)" for tool ids, for messages.
func Names(ids []string) string {
	var out []string
	for _, id := range ids {
		if t, ok := Lookup(id); ok {
			label := t.Name
			if t.Binary != "" && t.Binary != strings.ToLower(t.Name) {
				label += " (`" + t.Binary + "`)"
			}
			out = append(out, label)
		} else {
			out = append(out, id)
		}
	}
	return strings.Join(out, ", ")
}

// NewWithHost checks a described machine instead of this one, for tests: lookPath finds
// binaries, run answers their version commands. No SDK folders exist on it.
func NewWithHost(goos string, lookPath func(string) (string, error), run func(ctx context.Context, name string, args ...string) (string, error)) *Toolchain {
	c := &Checker{host: host{goos: goos, lookPath: lookPath, run: run, getenv: func(string) string { return "" },
		isDir: func(string) bool { return false }}, ttl: statusTTL, seen: map[string]Status{}}
	return &Toolchain{Checker: c, Installer: NewInstaller(c)}
}
