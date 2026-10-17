package stack

import "path"

// Package is one dependency manifest a repository installs from: which ecosystem it belongs to,
// the package manager that installs it and where it lives. The deps package turns these into
// install commands and dependency lists.
type Package struct {
	// Ecosystem is node, go, flutter, dart, ruby, python or jvm.
	Ecosystem string `json:"ecosystem"`
	// Manager is npm, pnpm, yarn, bun, go, flutter, dart, bundler, pip, maven or gradle.
	Manager string `json:"manager"`
	// Dir is the repository-relative folder holding the manifest ("." is the root).
	Dir string `json:"dir"`
	// Manifest is the repository-relative manifest file, e.g. "web/package.json".
	Manifest string `json:"manifest"`
	// Wrapper is set for a JVM build whose ./mvnw or ./gradlew is checked in.
	Wrapper bool `json:"wrapper,omitempty"`
}

// pythonManifests rank the files that mark a Python project; the first present names it.
var pythonManifests = []string{"pyproject.toml", "requirements.txt", "Pipfile"}

// addOtherManifest records Ruby and Python manifests, which have no stack rule of their own.
func (p *Profile) addOtherManifest(rel string) {
	base, dir := path.Base(rel), dirOf(rel)
	eco, manager := "", ""
	switch {
	case base == "Gemfile":
		eco, manager = "ruby", "bundler"
	case rank(pythonManifests, base) < len(pythonManifests):
		eco, manager = "python", "pip"
	default:
		return
	}
	for i, o := range p.others {
		if o.Ecosystem != eco || o.Dir != dir {
			continue
		}
		if rank(pythonManifests, base) < rank(pythonManifests, path.Base(o.Manifest)) {
			p.others[i].Manifest = rel
		}
		return
	}
	if len(p.others) < 2*maxDirs {
		p.others = append(p.others, Package{Ecosystem: eco, Manager: manager, Dir: dir, Manifest: rel})
	}
}

// Packages lists the repository's dependency manifests, shallowest first within each ecosystem.
// Node packages carry the manager the brief uses (packageManager field, then the nearest
// lockfile); JVM builds are build roots only, never nested modules.
func (p Profile) Packages() []Package {
	var out []Package
	for _, dir := range sortedKeys(p.nodePkgs) {
		out = append(out, Package{Ecosystem: "node", Manager: p.nodePkgs[dir].manager, Dir: dir, Manifest: joinRel(dir, "package.json")})
	}
	if t, ok := p.Get("go"); ok {
		for _, dir := range t.Dirs {
			out = append(out, Package{Ecosystem: "go", Manager: "go", Dir: dir, Manifest: joinRel(dir, "go.mod")})
		}
	}
	for _, d := range p.dartPkgs {
		eco := "dart"
		if d.flutter {
			eco = "flutter"
		}
		out = append(out, Package{Ecosystem: eco, Manager: eco, Dir: d.dir, Manifest: joinRel(d.dir, "pubspec.yaml")})
	}
	out = append(out, p.others...)
	for _, b := range p.jvmBuilds {
		manager := "maven"
		if b.gradle {
			manager = "gradle"
		}
		out = append(out, Package{Ecosystem: "jvm", Manager: manager, Dir: b.dir, Manifest: b.file, Wrapper: b.wrapper})
	}
	return out
}

func rank(list []string, v string) int {
	for i, x := range list {
		if x == v {
			return i
		}
	}
	return len(list)
}
