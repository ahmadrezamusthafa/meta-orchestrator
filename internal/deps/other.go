package deps

import (
	"encoding/xml"
	"path"
	"regexp"
	"strings"

	"github.com/ahmadrezamusthafa/meta-orchestrator/internal/stack"
)

const untrackedNote = "Whether these are installed is not checked; run the install to be sure."

// --- Ruby ---------------------------------------------------------------------------------

var gemLine = regexp.MustCompile(`^\s*gem\s+["']([^"']+)["']((?:\s*,\s*["'][^"']*["'])*)`)
var gemQuoted = regexp.MustCompile(`["']([^"']*)["']`)

func scanRuby(root string, p stack.Package) Ecosystem {
	e := Ecosystem{Ecosystem: "ruby", Manager: "bundler", Manifest: p.Manifest, Dir: p.Dir,
		Lockfile: firstExisting(root, p.Dir, "Gemfile.lock"),
		Install:  Install{Command: []string{"bundle", "install"}, Auto: true}}
	if e.Lockfile == "" {
		e.Install.Note = noLockfileNote
	}
	e.Install.Note = joinNote(e.Install.Note, untrackedNote, runsIn(p.Dir))

	locked := map[string]string{}
	if e.Lockfile != "" {
		for _, line := range strings.Split(string(readCapped(abs(root, e.Lockfile))), "\n") {
			// "    name (1.2.3)" — exactly four spaces: a resolved gem, not one of its requirements.
			if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "     ") {
				name, ver, ok := strings.Cut(strings.TrimSpace(line), " (")
				if ok {
					locked[name] = strings.TrimSuffix(ver, ")")
				}
			}
		}
	}
	depth, devDepth := 0, -1
	for _, line := range strings.Split(string(readCapped(abs(root, p.Manifest))), "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "group ") && strings.HasSuffix(t, " do"):
			depth++
			if devDepth < 0 && (strings.Contains(t, ":development") || strings.Contains(t, ":test")) {
				devDepth = depth
			}
			continue
		case strings.HasSuffix(t, " do") || t == "do":
			depth++
			continue
		case t == "end":
			if depth == devDepth {
				devDepth = -1
			}
			if depth > 0 {
				depth--
			}
			continue
		}
		m := gemLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		var wanted []string
		for _, q := range gemQuoted.FindAllStringSubmatch(m[2], -1) {
			wanted = append(wanted, q[1])
		}
		e.Dependencies = append(e.Dependencies, Dependency{Name: m[1], Wanted: strings.Join(wanted, ", "),
			Installed: locked[m[1]], Dev: devDepth > 0})
	}
	return e
}

// --- Python -------------------------------------------------------------------------------

var pyRequirement = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)(\[[^\]]*\])?\s*(.*)$`)

func scanPython(root string, p stack.Package) Ecosystem {
	e := Ecosystem{Ecosystem: "python", Manager: "pip", Manifest: p.Manifest, Dir: p.Dir}
	venv := relJoin(p.Dir, ".venv")
	hasVenv := isDir(abs(root, venv))
	switch {
	case firstExisting(root, p.Dir, "uv.lock") != "":
		e.Lockfile = relJoin(p.Dir, "uv.lock")
		e.Install = Install{Command: []string{"uv", "sync", "--frozen"}, Auto: true, Note: "Installs into the project's .venv."}
	case firstExisting(root, p.Dir, "poetry.lock") != "":
		e.Lockfile = relJoin(p.Dir, "poetry.lock")
		e.Install = Install{Command: []string{"poetry", "install", "--no-root"}, Auto: true, Note: "Installs into Poetry's virtual environment."}
	case firstExisting(root, p.Dir, "Pipfile.lock") != "":
		e.Lockfile = relJoin(p.Dir, "Pipfile.lock")
		e.Install = Install{Command: []string{"pipenv", "sync"}, Auto: true, Note: "Installs into Pipenv's virtual environment."}
	case firstExisting(root, p.Dir, "requirements.txt") != "" && hasVenv:
		e.Install = Install{Command: []string{".venv/bin/python", "-m", "pip", "install", "-r", "requirements.txt"}, Auto: true,
			Note: "Installs into the project's .venv."}
	case firstExisting(root, p.Dir, "requirements.txt") != "":
		e.Install = Install{Command: []string{"python3", "-m", "pip", "install", "-r", "requirements.txt"},
			Note: "No virtual environment found: create one first (python3 -m venv .venv) and install into it. The orchestrator does not install into the system Python."}
	default:
		e.Install = Install{Command: []string{"python3", "-m", "pip", "install", "-e", "."},
			Note: "No lockfile or virtual environment found: create a virtual environment first and install into it."}
	}
	e.Install.Note = joinNote(e.Install.Note, runsIn(p.Dir))
	e.Installed = hasVenv
	if e.Lockfile != "" && hasVenv {
		venvAt, _ := modTime(abs(root, venv))
		if lockAt, ok := modTime(abs(root, e.Lockfile)); ok && venvAt.Before(lockAt) {
			e.Installed = false
			e.Drift = ".venv is older than " + path.Base(e.Lockfile) + " — run " + strings.Join(e.Install.Command, " ") + " to restore the locked versions."
		}
	}

	locked := map[string]string{}
	if e.Lockfile != "" && !strings.HasSuffix(e.Lockfile, "Pipfile.lock") {
		name := ""
		for _, line := range strings.Split(string(readCapped(abs(root, e.Lockfile))), "\n") {
			t := strings.TrimSpace(line)
			switch {
			case t == "[[package]]":
				name = ""
			case strings.HasPrefix(t, "name = "):
				name = pyNorm(strings.Trim(strings.TrimPrefix(t, "name = "), `"`))
			case strings.HasPrefix(t, "version = ") && name != "":
				if _, seen := locked[name]; !seen {
					locked[name] = strings.Trim(strings.TrimPrefix(t, "version = "), `"`)
				}
			}
		}
	}
	add := func(spec string, dev bool) {
		spec = strings.TrimSpace(strings.SplitN(spec, ";", 2)[0])
		m := pyRequirement.FindStringSubmatch(spec)
		if m == nil {
			return
		}
		e.Dependencies = append(e.Dependencies, Dependency{Name: m[1], Wanted: strings.TrimSpace(m[3]), Installed: locked[pyNorm(m[1])], Dev: dev})
	}
	if req := firstExisting(root, p.Dir, "requirements.txt"); req != "" {
		for _, line := range strings.Split(string(readCapped(abs(root, req))), "\n") {
			if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "-") {
				add(t, false)
			}
		}
	} else if py := firstExisting(root, p.Dir, "pyproject.toml"); py != "" {
		for _, d := range pyprojectDeps(string(readCapped(abs(root, py)))) {
			add(d.spec, d.dev)
		}
	}
	return e
}

type pySpec struct {
	spec string
	dev  bool
}

// pyprojectDeps reads PEP 621 [project] dependencies and Poetry's dependency tables. It is a
// line reader, not a TOML parser: enough for the common layouts.
func pyprojectDeps(text string) []pySpec {
	var out []pySpec
	table, inList := "", false
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && !inList {
			table = strings.Trim(t, "[] ")
			continue
		}
		switch {
		case table == "project" && strings.HasPrefix(t, "dependencies") && strings.Contains(t, "["):
			inList = !strings.Contains(t, "]")
			for _, q := range gemQuoted.FindAllStringSubmatch(t, -1) {
				out = append(out, pySpec{spec: q[1]})
			}
		case inList:
			if strings.HasPrefix(t, "]") {
				inList = false
				continue
			}
			for _, q := range gemQuoted.FindAllStringSubmatch(t, -1) {
				out = append(out, pySpec{spec: q[1]})
			}
		case strings.HasPrefix(table, "tool.poetry.") && strings.Contains(table, "dependencies") && strings.Contains(t, "="):
			name, val, _ := strings.Cut(t, "=")
			name = strings.TrimSpace(name)
			if name == "python" {
				continue
			}
			ver := ""
			if q := gemQuoted.FindStringSubmatch(val); q != nil {
				ver = q[1]
			}
			out = append(out, pySpec{spec: name + " " + ver, dev: table != "tool.poetry.dependencies"})
		}
	}
	return out
}

func pyNorm(name string) string {
	return strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(name))
}

// --- JVM ----------------------------------------------------------------------------------

var gradleDep = regexp.MustCompile(`(?m)^\s*(implementation|api|compileOnly|runtimeOnly|annotationProcessor|kapt|ksp|testImplementation|testRuntimeOnly|androidTestImplementation|debugImplementation)\s*\(?\s*["']([^"':]+):([^"':]+):?([^"']*)["']`)

func scanJVM(root string, p stack.Package) Ecosystem {
	e := Ecosystem{Ecosystem: "jvm", Manager: p.Manager, Manifest: p.Manifest, Dir: p.Dir}
	switch {
	case p.Manager == "maven" && p.Wrapper:
		e.Install = Install{Command: []string{"./mvnw", "-q", "dependency:resolve"}, Auto: true}
	case p.Manager == "maven":
		e.Install = Install{Command: []string{"mvn", "-q", "dependency:resolve"},
			Note: "No ./mvnw wrapper is checked in, so the Maven version is not pinned: run this yourself."}
	case p.Wrapper:
		e.Install = Install{Command: []string{"./gradlew", "dependencies"}, Auto: true}
		e.Lockfile = firstExisting(root, p.Dir, "gradle.lockfile")
	default:
		e.Install = Install{Command: []string{"gradle", "dependencies"},
			Note: "No ./gradlew wrapper is checked in, so the Gradle version is not pinned: run this yourself."}
		e.Lockfile = firstExisting(root, p.Dir, "gradle.lockfile")
	}
	e.Install.Note = joinNote(e.Install.Note, untrackedNote, runsIn(p.Dir))
	text := string(readCapped(abs(root, p.Manifest)))
	if p.Manager == "maven" {
		e.Dependencies = pomDeps(text)
		return e
	}
	for _, m := range gradleDep.FindAllStringSubmatch(text, -1) {
		e.Dependencies = append(e.Dependencies, Dependency{Name: m[2] + ":" + m[3], Wanted: m[4],
			Dev: strings.Contains(strings.ToLower(m[1]), "test")})
	}
	return e
}

func pomDeps(text string) []Dependency {
	var pom struct {
		Properties struct {
			Entries []struct {
				XMLName xml.Name
				Value   string `xml:",chardata"`
			} `xml:",any"`
		} `xml:"properties"`
		Dependencies []struct {
			GroupID    string `xml:"groupId"`
			ArtifactID string `xml:"artifactId"`
			Version    string `xml:"version"`
			Scope      string `xml:"scope"`
		} `xml:"dependencies>dependency"`
	}
	if xml.Unmarshal([]byte(text), &pom) != nil {
		return nil
	}
	props := map[string]string{}
	for _, p := range pom.Properties.Entries {
		props[p.XMLName.Local] = strings.TrimSpace(p.Value)
	}
	var out []Dependency
	for _, d := range pom.Dependencies {
		ver := strings.TrimSpace(d.Version)
		if strings.HasPrefix(ver, "${") && strings.HasSuffix(ver, "}") {
			if v, ok := props[ver[2:len(ver)-1]]; ok {
				ver = v
			}
		}
		out = append(out, Dependency{Name: strings.TrimSpace(d.GroupID) + ":" + strings.TrimSpace(d.ArtifactID), Wanted: ver,
			Dev: d.Scope == "test"})
	}
	return out
}
