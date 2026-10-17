package packages

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/toolchain"
)

// system packages are installed by Homebrew (or another method of the toolchain catalog that runs
// without sudo). apt needs sudo, so it is only ever shown as a command to copy.

const systemRollbackNote = "System package managers keep no previous versions; use a version manager such as nvm/fnm for Node."

// systemPlan is how a system package can be installed here.
type systemPlan struct {
	argv    []string // nil when no unattended method exists
	manager string
	manual  Manual
}

func brewAvailable() bool {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return false
	}
	_, err := exec.LookPath("brew")
	return err == nil
}

// brewFormula is the manifest's formula, else the formula of the toolchain's Homebrew method.
func (m *Manager) brewFormula(s *Spec) string {
	if s.System.Brew != "" {
		return s.System.Brew
	}
	if t, ok := toolchain.Lookup(s.ToolchainID); ok {
		for _, meth := range t.Methods {
			if meth.Manager == "brew" && len(meth.Command) == 3 && meth.Command[1] == "install" {
				return meth.Command[2]
			}
		}
	}
	return ""
}

func (m *Manager) systemInstallPlan(s *Spec) systemPlan {
	p := systemPlan{manual: Manual{Commands: []string{}}}
	if s.ToolchainID != "" {
		rep := m.tc.Report([]toolchain.Requirement{{Tool: s.ToolchainID, Required: true}}, false)
		if len(rep.Items) == 1 {
			it := rep.Items[0]
			t, _ := toolchain.Lookup(s.ToolchainID)
			for _, mv := range it.Methods {
				if mv.Index == it.Recommended && mv.Auto && mv.Available && p.argv == nil {
					p.argv = append([]string(nil), t.Methods[mv.Index].Command...)
					p.manager = mv.Manager
				}
				if mv.Available || mv.Manager == "apt" || mv.Manager == "snap" {
					p.manual.Commands = append(p.manual.Commands, mv.Command)
					if mv.Note != "" && p.manual.Note == "" {
						p.manual.Note = mv.Note
					}
				}
			}
		}
	}
	if p.argv == nil && s.System.Brew != "" && brewAvailable() {
		p.argv, p.manager = []string{"brew", "install", s.System.Brew}, "brew"
	}
	if s.System.Brew != "" && !containsString(p.manual.Commands, "brew install "+s.System.Brew) {
		p.manual.Commands = append(p.manual.Commands, "brew install "+s.System.Brew)
	}
	if s.System.Apt != "" && runtime.GOOS == "linux" {
		c := "sudo apt-get install -y " + s.System.Apt
		if !containsString(p.manual.Commands, c) {
			p.manual.Commands = append(p.manual.Commands, c)
		}
	}
	if p.manual.Note == "" {
		if s.Docs != "" {
			p.manual.Note = "Install it from your terminal; see " + s.Docs
		} else {
			p.manual.Note = "Install it from your terminal."
		}
	}
	return p
}

func containsString(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// systemLock is the lock of the package manager a system install uses.
func (m *Manager) systemLock(s *Spec) string {
	if p := m.systemInstallPlan(s); p.manager != "" {
		return p.manager
	}
	return "brew"
}

func (m *Manager) systemManualError(s *Spec, what string) *ManualError {
	p := m.systemInstallPlan(s)
	reason := fmt.Sprintf("The orchestrator cannot %s %s unattended on this machine; run the command in your terminal.", what, s.Name)
	man := p.manual
	switch what {
	case "update":
		if f := m.brewFormula(s); f != "" {
			man = Manual{Commands: []string{"brew upgrade " + f}, Note: "Needs Homebrew (https://brew.sh)."}
		}
	case "uninstall":
		if f := m.brewFormula(s); f != "" {
			man = Manual{Commands: []string{"brew uninstall " + f}, Note: "Needs Homebrew (https://brew.sh)."}
		} else {
			man = Manual{Commands: []string{}, Note: "Uninstall it the way it was installed."}
		}
	}
	return &ManualError{Reason: reason, Manual: man}
}

// brewOwned reports whether the binary at path is a Homebrew install (it resolves into a
// Cellar or Caskroom). A copy from nvm, gvm, Docker Desktop or an installer must not be
// "updated" with brew: that would change a different copy than the one in use.
func brewOwned(path string) bool {
	if path == "" {
		return false
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	real = filepath.ToSlash(real)
	return strings.Contains(real, "/Cellar/") || strings.Contains(real, "/Caskroom/")
}

func (m *Manager) brewCommand(r *jobRun, verb, formula string) error {
	return r.run([]string{"brew", verb, formula}, nil, m.opts.Home)
}

func joinArgv(argv []string) string { return strings.Join(argv, " ") }
