package deps

import (
	"fmt"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
	"gopkg.in/yaml.v3"
)

func scanDart(root string, p stack.Package) Ecosystem {
	e := Ecosystem{Ecosystem: p.Ecosystem, Manager: p.Manager, Manifest: p.Manifest, Dir: p.Dir,
		Lockfile: firstExisting(root, p.Dir, "pubspec.lock"),
		Install:  Install{Command: []string{p.Manager, "pub", "get"}, Auto: true}}
	if e.Lockfile == "" {
		e.Install.Note = noLockfileNote
	}
	e.Install.Note = joinNote(e.Install.Note, runsIn(p.Dir))

	var spec struct {
		Dependencies    map[string]any `yaml:"dependencies"`
		DevDependencies map[string]any `yaml:"dev_dependencies"`
	}
	_ = yaml.Unmarshal(readCapped(abs(root, p.Manifest)), &spec)
	var lock struct {
		Packages map[string]struct {
			Version string `yaml:"version"`
		} `yaml:"packages"`
	}
	if e.Lockfile != "" {
		_ = yaml.Unmarshal(readCapped(abs(root, e.Lockfile)), &lock)
	}
	add := func(m map[string]any, dev bool) {
		for name, v := range m {
			wanted, sdk := pubConstraint(v)
			if sdk { // flutter, flutter_test: part of the SDK, not a package to install
				continue
			}
			e.Dependencies = append(e.Dependencies, Dependency{Name: name, Wanted: wanted, Installed: lock.Packages[name].Version, Dev: dev})
		}
	}
	add(spec.Dependencies, false)
	add(spec.DevDependencies, true)

	config := relJoin(p.Dir, ".dart_tool/package_config.json")
	configAt, ok := modTime(abs(root, config))
	if !ok {
		return e
	}
	e.Installed = true
	if e.Lockfile != "" {
		if lockAt, ok := modTime(abs(root, e.Lockfile)); ok && configAt.Before(lockAt) {
			e.Installed = false
			e.Drift = fmt.Sprintf(".dart_tool/package_config.json is older than pubspec.lock — run %s to restore the locked versions.",
				strings.Join(e.Install.Command, " "))
		}
	}
	return e
}

// pubConstraint renders a pubspec dependency value: a version constraint, or where it comes from.
func pubConstraint(v any) (wanted string, sdk bool) {
	switch x := v.(type) {
	case nil:
		return "any", false
	case string:
		return x, false
	case map[string]any:
		if _, ok := x["sdk"]; ok {
			return "", true
		}
		if ver, ok := x["version"].(string); ok {
			return ver, false
		}
		for _, k := range []string{"git", "path", "hosted"} {
			if _, ok := x[k]; ok {
				return k, false
			}
		}
	}
	return fmt.Sprint(v), false
}
