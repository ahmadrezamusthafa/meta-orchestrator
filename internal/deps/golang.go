package deps

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
)

func scanGo(root string, p stack.Package, modCache string) Ecosystem {
	e := Ecosystem{Ecosystem: "go", Manager: "go", Manifest: p.Manifest, Dir: p.Dir,
		Lockfile: firstExisting(root, p.Dir, "go.sum"),
		Install:  Install{Command: []string{"go", "mod", "download"}, Auto: true, Note: runsIn(p.Dir)}}
	missing := 0
	for _, r := range goRequires(string(readCapped(abs(root, p.Manifest)))) {
		d := Dependency{Name: r.path, Wanted: r.version, Dev: false}
		if modCache != "" && isDir(filepath.Join(modCache, escapeModule(r.path)+"@"+escapeModule(r.version))) {
			d.Installed = r.version
		} else {
			missing++
		}
		e.Dependencies = append(e.Dependencies, d)
	}
	e.Installed = missing == 0
	if missing > 0 && missing < len(e.Dependencies) {
		e.Drift = fmt.Sprintf("%d of %d required modules are not in the module cache — run go mod download.", missing, len(e.Dependencies))
	}
	return e
}

type goRequire struct{ path, version string }

// goRequires reads the require directives of a go.mod, direct and indirect.
func goRequires(text string) []goRequire {
	var out []goRequire
	inBlock := false
	for _, line := range strings.Split(text, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		switch {
		case len(f) == 0:
		case inBlock && f[0] == ")":
			inBlock = false
		case inBlock && len(f) >= 2:
			out = append(out, goRequire{f[0], f[1]})
		case f[0] == "require" && len(f) == 2 && f[1] == "(":
			inBlock = true
		case f[0] == "require" && len(f) >= 3:
			out = append(out, goRequire{f[1], f[2]})
		}
	}
	return out
}

// escapeModule applies the module cache's case encoding: "A" becomes "!a".
func escapeModule(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteByte('!')
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func goModCache() string {
	if c := os.Getenv("GOMODCACHE"); c != "" {
		return c
	}
	if gp := filepath.SplitList(os.Getenv("GOPATH")); len(gp) > 0 && gp[0] != "" {
		return filepath.Join(gp[0], "pkg", "mod")
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "go", "pkg", "mod")
	}
	return ""
}
