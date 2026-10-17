package deps

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
)

// Options tune a scan; the zero value checks this machine.
type Options struct {
	// LookPath finds a binary; nil uses exec.LookPath (this process's PATH).
	LookPath func(string) (string, error)
	// GoModCache is the Go module cache; "" resolves $GOMODCACHE, then $GOPATH/pkg/mod, then
	// ~/go/pkg/mod.
	GoModCache string
}

func (o Options) withDefaults() Options {
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.GoModCache == "" {
		o.GoModCache = goModCache()
	}
	return o
}

// toolFor maps a package manager binary to the toolchain entry that provides it.
var toolFor = map[string]string{
	"npm": "node", "pnpm": "pnpm", "yarn": "yarn", "bun": "bun", "go": "go", "flutter": "flutter",
	"dart": "dart", "mvn": "maven", "gradle": "gradle", "java": "java",
}

// Scan reports the dependency ecosystems of the repository at root: the shallowest manifest of
// each ecosystem, with its lockfile, install command, installed state and dependencies. It only
// reads files (and looks binaries up on PATH); it never runs a package manager.
func Scan(root string, opt Options) []Ecosystem {
	opt = opt.withDefaults()
	out := []Ecosystem{}
	if st, err := os.Stat(root); err != nil || !st.IsDir() {
		return out
	}
	seen := map[string]bool{}
	for _, p := range stack.Detect(root, stack.Quick).Packages() {
		if seen[p.Ecosystem] {
			continue
		}
		var e Ecosystem
		switch p.Ecosystem {
		case "node":
			e = scanNode(root, p)
		case "go":
			e = scanGo(root, p, opt.GoModCache)
		case "flutter", "dart":
			e = scanDart(root, p)
		case "ruby":
			e = scanRuby(root, p)
		case "python":
			e = scanPython(root, p)
		case "jvm":
			e = scanJVM(root, p)
		default:
			continue
		}
		seen[p.Ecosystem] = true
		resolveBinary(root, &e, opt.LookPath)
		markKeys(&e)
		sortDeps(e.Dependencies)
		if e.Dependencies == nil {
			e.Dependencies = []Dependency{}
		}
		out = append(out, e)
	}
	return out
}

// Find returns the ecosystem named eco from a scan.
func Find(ecos []Ecosystem, eco string) (Ecosystem, bool) {
	for _, e := range ecos {
		if e.Ecosystem == eco {
			return e, true
		}
	}
	return Ecosystem{}, false
}

// resolveBinary decides what Install.Command[0] runs and whether it exists.
func resolveBinary(root string, e *Ecosystem, lookPath func(string) (string, error)) {
	if len(e.Install.Command) == 0 {
		return
	}
	name := e.Install.Command[0]
	if strings.HasPrefix(name, "./") || strings.HasPrefix(name, ".venv/") {
		bin := filepath.Join(root, filepath.FromSlash(e.Dir), filepath.FromSlash(name))
		if !isFile(bin) {
			e.Missing = true
			return
		}
		e.bin = bin
		if strings.HasPrefix(name, "./") { // mvnw and gradlew need a JDK
			if _, err := lookPath("java"); err != nil {
				e.Missing, e.MissingTool = true, "java"
			}
		}
		return
	}
	bin, err := lookPath(name)
	if err != nil {
		e.Missing, e.MissingTool = true, toolFor[name]
		return
	}
	e.bin = bin
}

// Binary is the resolved executable for the ecosystem's install command ("" when missing).
func (e Ecosystem) Binary() string { return e.bin }

// Runnable reports whether the orchestrator may run the install command itself.
func (e Ecosystem) Runnable() bool { return e.Install.Auto && !e.Missing && e.bin != "" }

func sortDeps(ds []Dependency) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].Dev != ds[j].Dev {
			return !ds[i].Dev
		}
		return ds[i].Name < ds[j].Name
	})
}

func runsIn(dir string) string {
	if dir == "." || dir == "" {
		return ""
	}
	return "Runs in " + dir + "/."
}

func joinNote(parts ...string) string {
	var out []string
	for _, p := range parts {
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

func relJoin(dir, name string) string {
	if dir == "." || dir == "" {
		return name
	}
	return path.Join(dir, name)
}

func abs(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func modTime(p string) (time.Time, bool) {
	st, err := os.Stat(p)
	if err != nil {
		return time.Time{}, false
	}
	return st.ModTime(), true
}

// readCapped reads at most 8 MiB of a file: manifests and lockfiles, never anything huge.
func readCapped(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	b := make([]byte, 0, 64<<10)
	buf := make([]byte, 32<<10)
	for len(b) < 8<<20 {
		n, err := f.Read(buf)
		b = append(b, buf[:n]...)
		if err != nil {
			break
		}
	}
	return b
}

// firstExisting returns the first of names present in dir (repository-relative), or "".
func firstExisting(root, dir string, names ...string) string {
	for _, n := range names {
		if rel := relJoin(dir, n); isFile(abs(root, rel)) {
			return rel
		}
	}
	return ""
}

// listMismatch turns "name: a vs b" lines into one sentence with at most three examples.
func listMismatch(items []string) string {
	if len(items) == 0 {
		return ""
	}
	sort.Strings(items)
	shown := items
	more := ""
	if len(items) > 3 {
		shown = items[:3]
		more = " and " + strconv.Itoa(len(items)-3) + " more"
	}
	return strings.Join(shown, "; ") + more
}
