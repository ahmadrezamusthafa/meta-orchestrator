package deps

import (
	"fmt"
	"regexp"
	"strings"
)

// companions are packages that usually have to move together with the one being upgraded.
var companions = map[string]string{
	"react":         "react-dom, @types/react and @types/react-dom",
	"react-dom":     "react, @types/react and @types/react-dom",
	"vue":           "vue-router, pinia, @vue/test-utils and @vitejs/plugin-vue",
	"next":          "react, react-dom and eslint-config-next",
	"@angular/core": "every other @angular/* package (ng update keeps them aligned)",
	"typescript":    "@typescript-eslint/* and ts-node / tsx",
	"vite":          "the @vitejs/* plugins and vitest",
}

var (
	safeDepName = regexp.MustCompile(`^[A-Za-z0-9@][A-Za-z0-9@/._:~+-]{0,213}$`)
	safeTarget  = regexp.MustCompile(`^[A-Za-z0-9.^~<>=*+_ |:/@-]{1,100}$`)
)

// ValidUpgrade checks a dependency name and target version before they go into a task.
func ValidUpgrade(name, target string) error {
	if !safeDepName.MatchString(name) {
		return fmt.Errorf("invalid dependency name %q", name)
	}
	if !safeTarget.MatchString(strings.TrimSpace(target)) {
		return fmt.Errorf("invalid target version %q", target)
	}
	return nil
}

// UpgradeTitle names the task that upgrades a dependency.
func UpgradeTitle(name, target, repo string) string {
	return fmt.Sprintf("Upgrade %s to %s in %s", name, target, repo)
}

// UpgradeDescription spells out how the agent upgrades dep to target in the ecosystem: the
// manager command that updates manifest and lockfile together, then breaking changes and checks.
func UpgradeDescription(e Ecosystem, dep Dependency, target, repo, note string) string {
	var b strings.Builder
	from := dep.Installed
	if from == "" {
		from = dep.Wanted
	}
	fmt.Fprintf(&b, "Upgrade the %s dependency `%s` from %s to %s in repository `%s`.\n\n", ecoLabel(e), dep.Name, orUnknown(from), target, repo)
	fmt.Fprintf(&b, "Manifest: `%s`", e.Manifest)
	if e.Lockfile != "" {
		fmt.Fprintf(&b, " · Lockfile: `%s`", e.Lockfile)
	}
	fmt.Fprintf(&b, " · Package manager: %s\n\n", e.Manager)
	b.WriteString("Steps:\n")
	cmd := upgradeCommand(e, dep, target)
	where := ""
	if e.Dir != "." && e.Dir != "" {
		where = fmt.Sprintf(" (in `%s/`)", e.Dir)
	}
	if cmd != "" {
		fmt.Fprintf(&b, "1. Update the manifest and the lockfile together with the repository's own package manager%s: `%s`. Do not hand-edit the lockfile.\n", where, cmd)
	} else {
		fmt.Fprintf(&b, "1. Change the version of `%s` in `%s` to %s%s, then regenerate the lockfile with %s. Do not hand-edit the lockfile.\n", dep.Name, e.Manifest, target, where, e.Manager)
	}
	if c := companions[dep.Name]; c != "" && e.Ecosystem == "node" {
		fmt.Fprintf(&b, "2. Move the packages that must match it as well: %s.\n", c)
	} else {
		b.WriteString("2. Move any packages that must match it (peer dependencies, type packages, plugins) as well.\n")
	}
	fmt.Fprintf(&b, "3. Read the release notes / migration guide between %s and %s and fix every breaking change in the code.\n", orUnknown(from), target)
	b.WriteString("4. Run the repository's checks (build, lint, type check, tests) and make them pass.\n")
	b.WriteString("5. Keep the change limited to this upgrade; the pull request is the review, and reverting it is the rollback.\n")
	if note = strings.TrimSpace(note); note != "" {
		fmt.Fprintf(&b, "\nOperator note: %s\n", note)
	}
	return b.String()
}

func upgradeCommand(e Ecosystem, dep Dependency, target string) string {
	spec := dep.Name + "@" + target
	switch e.Manager {
	case "npm":
		if dep.Dev {
			return "npm install --save-dev " + spec
		}
		return "npm install " + spec
	case "pnpm":
		return "pnpm add " + flagIf(dep.Dev, "-D ") + spec
	case "yarn":
		return "yarn add " + flagIf(dep.Dev, "-D ") + spec
	case "bun":
		return "bun add " + flagIf(dep.Dev, "-d ") + spec
	case "go":
		return "go get " + spec + " && go mod tidy"
	case "flutter", "dart":
		dev := ""
		if dep.Dev {
			dev = "dev:"
		}
		return e.Manager + " pub add '" + dev + dep.Name + ":" + target + "'"
	}
	return ""
}

func flagIf(on bool, flag string) string {
	if on {
		return flag
	}
	return ""
}

func ecoLabel(e Ecosystem) string {
	switch e.Ecosystem {
	case "node":
		return "Node.js"
	case "go":
		return "Go"
	case "flutter":
		return "Flutter"
	case "dart":
		return "Dart"
	case "ruby":
		return "Ruby"
	case "python":
		return "Python"
	case "jvm":
		return "JVM"
	}
	return e.Ecosystem
}

func orUnknown(s string) string {
	if s == "" {
		return "its current version"
	}
	return s
}
