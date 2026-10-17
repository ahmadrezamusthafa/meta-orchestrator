package deps

import (
	"encoding/json"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
	"gopkg.in/yaml.v3"
)

const noLockfileNote = "No lockfile — versions may drift."

// nodeLockfiles per manager, in the order the manager itself prefers them.
var nodeLockfiles = map[string][]string{
	"npm":  {"npm-shrinkwrap.json", "package-lock.json"},
	"pnpm": {"pnpm-lock.yaml"},
	"yarn": {"yarn.lock"},
	"bun":  {"bun.lock", "bun.lockb"},
}

// nodeMarkers are the files a manager writes into node_modules after an install; their age
// compared with the lockfile tells whether node_modules is current.
var nodeMarkers = map[string]string{
	"npm": ".package-lock.json", "pnpm": ".modules.yaml", "yarn": ".yarn-integrity", "yarn-berry": ".yarn-state.yml",
}

type packageJSON struct {
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	PackageManager  string            `json:"packageManager"`
}

func scanNode(root string, p stack.Package) Ecosystem {
	e := Ecosystem{Ecosystem: "node", Manager: p.Manager, Manifest: p.Manifest, Dir: p.Dir}
	if _, ok := nodeLockfiles[e.Manager]; !ok {
		e.Manager = "npm"
	}
	var pkg packageJSON
	_ = json.Unmarshal(readCapped(abs(root, p.Manifest)), &pkg)

	// A workspace member installs from the workspace root, where the lockfile lives.
	for _, dir := range []string{p.Dir, "."} {
		if l := firstExisting(root, dir, nodeLockfiles[e.Manager]...); l != "" {
			e.Lockfile, e.Dir = l, dir
			break
		}
	}
	berry := e.Manager == "yarn" && yarnBerry(root, p.Dir, e.Lockfile, pkg.PackageManager)

	switch {
	case e.Lockfile == "":
		e.Install = Install{Command: []string{e.Manager, "install"}, Auto: true, Note: noLockfileNote}
	case e.Manager == "npm":
		e.Install = Install{Command: []string{"npm", "ci"}, Auto: true}
	case e.Manager == "pnpm":
		e.Install = Install{Command: []string{"pnpm", "install", "--frozen-lockfile"}, Auto: true}
	case e.Manager == "yarn" && berry:
		e.Install = Install{Command: []string{"yarn", "install", "--immutable"}, Auto: true}
	case e.Manager == "yarn":
		e.Install = Install{Command: []string{"yarn", "install", "--frozen-lockfile"}, Auto: true}
	case e.Manager == "bun":
		e.Install = Install{Command: []string{"bun", "install", "--frozen-lockfile"}, Auto: true}
	}
	e.Install.Note = joinNote(e.Install.Note, runsIn(e.Dir))

	locked := nodeLocked(root, e, p.Dir)
	nmDirs := []string{relJoin(p.Dir, "node_modules")}
	if e.Dir != p.Dir {
		nmDirs = append(nmDirs, relJoin(e.Dir, "node_modules"))
	}
	var mismatches []string
	add := func(m map[string]string, dev bool) {
		for name, wanted := range m {
			inst := nodeModuleVersion(root, nmDirs, name)
			lock := locked(name, wanted)
			if inst != "" && lock != "" && inst != lock {
				mismatches = append(mismatches, fmt.Sprintf("%s: node_modules has %s, the lockfile pins %s", name, inst, lock))
			}
			if inst == "" {
				inst = lock
			}
			e.Dependencies = append(e.Dependencies, Dependency{Name: name, Wanted: wanted, Installed: inst, Dev: dev})
		}
	}
	add(pkg.Dependencies, false)
	add(pkg.DevDependencies, true)

	nm := abs(root, relJoin(e.Dir, "node_modules"))
	if !isDir(nm) {
		return e
	}
	e.Installed = true
	if e.Lockfile != "" {
		marker := e.Manager
		if berry {
			marker = "yarn-berry"
		}
		installedAt, ok := modTime(path.Join(nm, nodeMarkers[marker]))
		if !ok || nodeMarkers[marker] == "" {
			installedAt, _ = modTime(nm)
		}
		if lockAt, ok := modTime(abs(root, e.Lockfile)); ok && installedAt.Before(lockAt) {
			e.Installed = false
			e.Drift = fmt.Sprintf("node_modules is older than %s — run %s to restore the locked versions.",
				path.Base(e.Lockfile), strings.Join(e.Install.Command, " "))
		}
	}
	if len(mismatches) > 0 {
		e.Installed = false
		e.Drift = joinNote(e.Drift, "Installed versions differ from the lockfile: "+listMismatch(mismatches)+".")
	}
	return e
}

// yarnBerry reports Yarn 2+ (Berry), which takes --immutable instead of --frozen-lockfile.
func yarnBerry(root, dir, lockfile, packageManager string) bool {
	if v, ok := strings.CutPrefix(packageManager, "yarn@"); ok {
		major, _ := strconv.Atoi(strings.SplitN(v, ".", 2)[0])
		return major >= 2
	}
	if firstExisting(root, dir, ".yarnrc.yml") != "" || firstExisting(root, ".", ".yarnrc.yml") != "" {
		return true
	}
	return lockfile != "" && strings.Contains(string(readCapped(abs(root, lockfile))), "\n__metadata:")
}

// nodeModuleVersion reads node_modules/<name>/package.json in the first folder that has it.
func nodeModuleVersion(root string, nmDirs []string, name string) string {
	for _, d := range nmDirs {
		b := readCapped(abs(root, path.Join(d, name, "package.json")))
		if b == nil {
			continue
		}
		var m struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(b, &m) == nil && m.Version != "" {
			return m.Version
		}
	}
	return ""
}

// nodeLocked returns a lookup of the version the lockfile pins for a direct dependency.
func nodeLocked(root string, e Ecosystem, pkgDir string) func(name, wanted string) string {
	none := func(string, string) string { return "" }
	if e.Lockfile == "" || strings.HasSuffix(e.Lockfile, ".lockb") {
		return none
	}
	data := readCapped(abs(root, e.Lockfile))
	member := "" // the package's path inside the workspace, "" at the workspace root
	if pkgDir != e.Dir {
		member = strings.TrimPrefix(pkgDir, e.Dir+"/")
		if e.Dir == "." {
			member = pkgDir
		}
	}
	switch path.Base(e.Lockfile) {
	case "package-lock.json", "npm-shrinkwrap.json":
		var lock struct {
			Packages     map[string]struct{ Version string } `json:"packages"`
			Dependencies map[string]struct{ Version string } `json:"dependencies"`
		}
		if json.Unmarshal(data, &lock) != nil {
			return none
		}
		return func(name, _ string) string {
			if member != "" {
				if p, ok := lock.Packages[member+"/node_modules/"+name]; ok && p.Version != "" {
					return p.Version
				}
			}
			if p, ok := lock.Packages["node_modules/"+name]; ok && p.Version != "" {
				return p.Version
			}
			return lock.Dependencies[name].Version
		}
	case "pnpm-lock.yaml":
		return pnpmLocked(data, member)
	case "yarn.lock":
		return yarnLocked(data)
	case "bun.lock":
		text := string(data)
		return func(name, _ string) string {
			key := `"` + name + `": ["` + name + `@`
			i := strings.Index(text, key)
			if i < 0 {
				return ""
			}
			rest := text[i+len(key):]
			if j := strings.IndexByte(rest, '"'); j > 0 {
				return rest[:j]
			}
			return ""
		}
	}
	return none
}

func pnpmLocked(data []byte, member string) func(name, wanted string) string {
	type section struct {
		Dependencies    map[string]any `yaml:"dependencies"`
		DevDependencies map[string]any `yaml:"devDependencies"`
	}
	var lock struct {
		section   `yaml:",inline"`
		Importers map[string]section `yaml:"importers"`
	}
	if yaml.Unmarshal(data, &lock) != nil {
		return func(string, string) string { return "" }
	}
	importer := member
	if importer == "" {
		importer = "."
	}
	sec := lock.section
	if imp, ok := lock.Importers[importer]; ok {
		sec = imp
	}
	return func(name, _ string) string {
		v, ok := sec.Dependencies[name]
		if !ok {
			v = sec.DevDependencies[name]
		}
		var s string
		switch x := v.(type) {
		case string:
			s = x
		case map[string]any:
			s, _ = x["version"].(string)
		}
		if i := strings.IndexByte(s, '('); i > 0 {
			s = s[:i]
		}
		if i := strings.IndexByte(s, '_'); i > 0 { // lockfile v5 peer suffix
			s = s[:i]
		}
		if strings.HasPrefix(s, "link:") || strings.HasPrefix(s, "file:") {
			return ""
		}
		return s
	}
}

// yarnLocked parses yarn.lock (v1 and Berry) into spec → version.
func yarnLocked(data []byte) func(name, wanted string) string {
	versions := map[string]string{}
	var specs []string
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.HasSuffix(line, ":") {
			specs = specs[:0]
			for _, s := range strings.Split(strings.TrimSuffix(line, ":"), ",") {
				specs = append(specs, strings.Trim(strings.TrimSpace(s), `"`))
			}
			continue
		}
		t := strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(t, "version "); ok && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			for _, s := range specs {
				versions[s] = strings.Trim(v, `"`)
			}
		} else if v, ok := strings.CutPrefix(t, "version: "); ok && !strings.HasPrefix(line, "    ") {
			for _, s := range specs {
				versions[s] = strings.Trim(v, `"`)
			}
		}
	}
	return func(name, wanted string) string {
		for _, k := range []string{name + "@" + wanted, name + "@npm:" + wanted} {
			if v, ok := versions[k]; ok {
				return v
			}
		}
		return ""
	}
}
