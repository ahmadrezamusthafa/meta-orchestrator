package stack

import (
	"path"
	"strings"
)

// Need is a command-line tool this machine needs to verify changes in the repository. Tool is a
// toolchain catalog id (internal/toolchain); Required tools block a code stage until they are
// installed or the operator continues without them, recommended ones only weaken the checks.
type Need struct {
	Tool     string `json:"tool"`
	Required bool   `json:"required"`
	Why      string `json:"why"`
}

// Needs lists the tools the brief's checks (see checks) rely on, in stack order.
func (p Profile) Needs() []Need {
	var out []Need
	add := func(tool string, required bool, why string) {
		for i, n := range out {
			if n.Tool == tool {
				out[i].Required = n.Required || required
				return
			}
		}
		out = append(out, Need{Tool: tool, Required: required, Why: why})
	}

	if p.Has("go") {
		add("go", true, "build and test the Go code")
		if len(p.lints) > 0 {
			add("golangci-lint", false, "run the repository's golangci-lint config")
		}
	}

	node := len(p.nodeChecks()) > 0
	if node {
		add("node", true, "run the package's typecheck, lint and test scripts")
		for _, dir := range sortedKeys(p.nodePkgs) {
			if m := p.nodePkgs[dir].manager; m != "npm" && p.nodeDirUsed(dir) {
				add(m, true, "the package is managed with "+m)
			}
		}
	}
	if !node && p.Has("openapi") {
		add("node", false, "lint the OpenAPI spec through npx")
	}
	if !node && p.usesWrangler() {
		add("node", false, "dry-run the Workers deploy through npx wrangler")
	}

	if p.Has("java") || p.Has("kotlin") || p.Has("android") {
		add("java", true, "build and test the JVM code")
		for _, b := range p.jvmBuilds {
			switch {
			case b.wrapper:
			case b.gradle:
				add("gradle", true, "the Gradle build has no ./gradlew wrapper")
			default:
				add("maven", true, "the Maven build has no ./mvnw wrapper")
			}
		}
	}
	if p.Has("android") {
		add("android-sdk", true, "build the Android app")
	}
	for _, d := range p.dartPkgs {
		if d.flutter {
			add("flutter", true, "analyze and test the Flutter app")
		} else {
			add("dart", true, "analyze and test the Dart package")
		}
	}

	if p.Has("opentofu") {
		add("tofu", true, "format-check and validate the OpenTofu modules")
	}
	if p.Has("terragrunt") {
		add("terragrunt", true, "format-check the Terragrunt configuration")
	}
	if t, ok := p.Get("spicedb"); ok {
		for _, f := range t.Files {
			if strings.HasSuffix(f, ".zed") {
				add("zed", false, "validate the SpiceDB schema")
				break
			}
		}
	}
	if p.Has("github-actions") {
		add("actionlint", false, "lint the GitHub Actions workflows")
	}
	if p.Has("docker") {
		add("docker", false, "build the container image when the Dockerfile changes")
	}
	return out
}

// nodeDirUsed reports whether the package folder holds a technology the brief runs checks for.
func (p Profile) nodeDirUsed(dir string) bool {
	for _, id := range []string{"typescript", "react", "playwright"} {
		if t, ok := p.Get(id); ok {
			for _, d := range t.Dirs {
				if d == dir {
					return true
				}
			}
		}
	}
	return false
}

// usesWrangler reports whether Cloudflare is managed with wrangler config, not only through IaC.
func (p Profile) usesWrangler() bool {
	if t, ok := p.Get("cloudflare"); ok {
		for _, f := range t.Files {
			if strings.HasPrefix(path.Base(f), "wrangler.") {
				return true
			}
		}
	}
	return false
}
