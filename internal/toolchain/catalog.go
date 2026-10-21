// Package toolchain knows the command-line tools the supported tech stacks need on this machine:
// how to tell whether each is really installed (by running it, not just finding it on PATH), and
// how to install it — one click when the package manager can do it unattended, a copyable command
// when it needs a password or a GUI, and the official docs otherwise.
//
// Adding a tool is one Tool entry below; a stack asks for it by id from stack.Profile.Needs.
package toolchain

import "time"

// Method is one way to install a tool.
type Method struct {
	// Manager runs the command: brew, npm, go, apt or snap. It must be present on this machine.
	Manager string `json:"manager"`
	// Command is run exactly as written — never through a shell, never with operator input.
	Command []string `json:"command"`
	// Auto marks a command the orchestrator may run for the operator: no sudo, no password
	// prompt, no GUI. Other methods are shown for the operator to run in their own terminal.
	Auto bool `json:"auto"`
	// OS limits the method to these GOOS values; empty means wherever Manager exists.
	OS   []string `json:"os,omitempty"`
	Note string   `json:"note,omitempty"`
}

// Tool is a command-line tool a stack needs.
type Tool struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Docs    string `json:"docs"`
	// Binary is looked up on PATH and run with VersionArgs; it counts as installed only when that
	// succeeds (macOS ships a /usr/bin/java stub that fails without a JDK).
	Binary      string   `json:"binary,omitempty"`
	VersionArgs []string `json:"-"`
	// SDKEnv/SDKDirs locate a tool that is a folder rather than a command (the Android SDK): the
	// first environment variable naming an existing folder, else the first default folder
	// (relative to the home folder) that exists.
	SDKEnv  []string `json:"-"`
	SDKDirs []string `json:"-"`
	// Timeout bounds the version check; zero uses defaultCheckTimeout.
	Timeout time.Duration `json:"-"`
	Methods []Method      `json:"-"`
}

const defaultCheckTimeout = 8 * time.Second

var (
	macOS     = []string{"darwin"}
	linux     = []string{"linux"}
	brewHosts = []string{"darwin", "linux"} // Homebrew also runs on Linux
)

func brew(args ...string) Method {
	return Method{Manager: "brew", Command: append([]string{"brew", "install"}, args...), Auto: true, OS: brewHosts}
}

func npmGlobal(pkg string) Method {
	return Method{Manager: "npm", Command: []string{"npm", "install", "-g", pkg}, Auto: true,
		Note: "Fails with EACCES when Node was installed system-wide; then run it with a Node version manager (nvm, fnm) or install another way."}
}

func apt(pkgs ...string) Method {
	return Method{Manager: "apt", Command: append([]string{"sudo", "apt-get", "install", "-y"}, pkgs...), OS: linux,
		Note: "Needs sudo, so run it in your terminal."}
}

// Catalog lists every tool the orchestrator can check and install.
var Catalog = []Tool{
	{ID: "go", Name: "Go", Purpose: "Builds and tests Go services.", Docs: "https://go.dev/doc/install",
		Binary: "go", VersionArgs: []string{"version"},
		Methods: []Method{brew("go"), withNote(apt("golang-go"), "Needs sudo. Ubuntu's package can lag behind the go.mod version; go.dev has the latest release.")}},
	{ID: "golangci-lint", Name: "golangci-lint", Purpose: "Runs the repository's Go linters.", Docs: "https://golangci-lint.run/welcome/install/",
		Binary: "golangci-lint", VersionArgs: []string{"--version"},
		Methods: []Method{brew("golangci-lint")}},
	{ID: "node", Name: "Node.js", Purpose: "Runs JavaScript/TypeScript tooling and npx commands.", Docs: "https://nodejs.org/en/download",
		Binary: "node", VersionArgs: []string{"--version"},
		Methods: []Method{brew("node"), apt("nodejs", "npm")}},
	{ID: "pnpm", Name: "pnpm", Purpose: "Package manager the repository uses.", Docs: "https://pnpm.io/installation",
		Binary: "pnpm", VersionArgs: []string{"--version"},
		Methods: []Method{npmGlobal("pnpm"), brew("pnpm")}},
	{ID: "yarn", Name: "Yarn", Purpose: "Package manager the repository uses.", Docs: "https://yarnpkg.com/getting-started/install",
		Binary: "yarn", VersionArgs: []string{"--version"},
		Methods: []Method{npmGlobal("yarn"), brew("yarn")}},
	{ID: "bun", Name: "Bun", Purpose: "Package manager and runtime the repository uses.", Docs: "https://bun.sh/docs/installation",
		Binary: "bun", VersionArgs: []string{"--version"},
		Methods: []Method{brew("oven-sh/bun/bun"), npmGlobal("bun")}},
	{ID: "java", Name: "Java (JDK)", Purpose: "Compiles and tests Java, Kotlin and Android code.", Docs: "https://adoptium.net/installation/",
		Binary: "java", VersionArgs: []string{"-version"},
		Methods: []Method{
			{Manager: "brew", Command: []string{"brew", "install", "--cask", "temurin"}, OS: macOS,
				Note: "The installer asks for your macOS password, so run it in your terminal."},
			apt("default-jdk"),
		}},
	{ID: "maven", Name: "Maven", Purpose: "Builds Maven projects that have no ./mvnw wrapper.", Docs: "https://maven.apache.org/install.html",
		Binary: "mvn", VersionArgs: []string{"-v"},
		Methods: []Method{brew("maven"), apt("maven")}},
	{ID: "gradle", Name: "Gradle", Purpose: "Builds Gradle projects that have no ./gradlew wrapper.", Docs: "https://gradle.org/install/",
		Binary: "gradle", VersionArgs: []string{"--version"}, Timeout: 30 * time.Second,
		Methods: []Method{brew("gradle")}},
	{ID: "android-sdk", Name: "Android SDK", Purpose: "Builds Android apps with Gradle.", Docs: "https://developer.android.com/studio",
		SDKEnv: []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"}, SDKDirs: []string{"Library/Android/sdk", "Android/Sdk"},
		Methods: []Method{{Manager: "brew", Command: []string{"brew", "install", "--cask", "android-studio"}, OS: macOS,
			Note: "Open Android Studio once afterwards so it downloads the SDK, or set ANDROID_HOME to an existing SDK."}}},
	{ID: "flutter", Name: "Flutter", Purpose: "Analyzes, tests and builds Flutter apps (includes Dart).", Docs: "https://docs.flutter.dev/get-started/install",
		Binary: "flutter", VersionArgs: []string{"--version"}, Timeout: 60 * time.Second,
		Methods: []Method{
			{Manager: "brew", Command: []string{"brew", "install", "--cask", "flutter"}, Auto: true, OS: macOS},
			{Manager: "snap", Command: []string{"sudo", "snap", "install", "flutter", "--classic"}, OS: linux, Note: "Needs sudo, so run it in your terminal."},
		}},
	{ID: "dart", Name: "Dart SDK", Purpose: "Analyzes and tests Dart packages.", Docs: "https://dart.dev/get-dart",
		Binary: "dart", VersionArgs: []string{"--version"},
		Methods: []Method{brew("dart-lang/dart/dart")}},
	{ID: "tofu", Name: "OpenTofu", Purpose: "Formats and validates OpenTofu modules.", Docs: "https://opentofu.org/docs/intro/install/",
		Binary: "tofu", VersionArgs: []string{"version"},
		Methods: []Method{brew("opentofu"),
			{Manager: "snap", Command: []string{"sudo", "snap", "install", "--classic", "opentofu"}, OS: linux, Note: "Needs sudo, so run it in your terminal."}}},
	{ID: "terragrunt", Name: "Terragrunt", Purpose: "Formats and runs Terragrunt configuration.", Docs: "https://terragrunt.gruntwork.io/docs/getting-started/install/",
		Binary: "terragrunt", VersionArgs: []string{"--version"},
		Methods: []Method{brew("terragrunt")}},
	{ID: "zed", Name: "zed (SpiceDB CLI)", Purpose: "Validates SpiceDB schemas.", Docs: "https://authzed.com/docs/spicedb/getting-started/installing-zed",
		Binary: "zed", VersionArgs: []string{"version"},
		Methods: []Method{brew("authzed/tap/zed")}},
	{ID: "nginx", Name: "Nginx", Purpose: "Tests gateway configuration with nginx -t.", Docs: "https://nginx.org/en/docs/install.html",
		Binary: "nginx", VersionArgs: []string{"-v"},
		Methods: []Method{brew("nginx"), apt("nginx")}},
	{ID: "openresty", Name: "OpenResty", Purpose: "Tests OpenResty gateway configuration, including Lua directives, with openresty -t.", Docs: "https://openresty.org/en/installation.html",
		Binary: "openresty", VersionArgs: []string{"-v"},
		Methods: []Method{brew("openresty/brew/openresty")}},
	{ID: "luacheck", Name: "luacheck", Purpose: "Lints Lua code.", Docs: "https://luacheck.readthedocs.io/en/stable/install.html",
		Binary: "luacheck", VersionArgs: []string{"--version"},
		Methods: []Method{brew("luacheck")}},
	{ID: "actionlint", Name: "actionlint", Purpose: "Lints GitHub Actions workflows.", Docs: "https://github.com/rhysd/actionlint/blob/main/docs/install.md",
		Binary: "actionlint", VersionArgs: []string{"--version"},
		Methods: []Method{brew("actionlint"),
			{Manager: "go", Command: []string{"go", "install", "github.com/rhysd/actionlint/cmd/actionlint@latest"}, Auto: true}}},
	{ID: "github-mcp-server", Name: "GitHub MCP server", Purpose: "Runs the GitHub connector for agents without Docker.",
		Docs: "https://github.com/github/github-mcp-server", Binary: "github-mcp-server", VersionArgs: []string{"--version"},
		Methods: []Method{{Manager: "go", Command: []string{"go", "install", "github.com/github/github-mcp-server/cmd/github-mcp-server@latest"}, Auto: true}}},
	{ID: "mcp-grafana", Name: "Grafana MCP server", Purpose: "Runs the Grafana connector for agents without Docker.",
		Docs: "https://github.com/grafana/mcp-grafana", Binary: "mcp-grafana", VersionArgs: []string{"--version"},
		Methods: []Method{{Manager: "go", Command: []string{"go", "install", "github.com/grafana/mcp-grafana/cmd/mcp-grafana@latest"}, Auto: true}}},
	{ID: "azure-cli", Name: "Azure CLI", Purpose: "Signs the Azure connector in (`az login`).", Docs: "https://learn.microsoft.com/cli/azure/install-azure-cli",
		Binary: "az", VersionArgs: []string{"version"}, Timeout: 30 * time.Second,
		Methods: []Method{brew("azure-cli")}},
	{ID: "docker", Name: "Docker", Purpose: "Builds and runs containers.", Docs: "https://docs.docker.com/get-started/get-docker/",
		Binary: "docker", VersionArgs: []string{"--version"},
		Methods: []Method{withNote(apt("docker.io"), "Needs sudo. On macOS install Docker Desktop, OrbStack or Colima from their sites.")}},

	// Launch readiness (internal/readiness): the checks its stages run and wire into pipelines.
	{ID: "gitleaks", Name: "Gitleaks", Purpose: "Scans the repository and its history for committed secrets.", Docs: "https://github.com/gitleaks/gitleaks#installing",
		Binary: "gitleaks", VersionArgs: []string{"version"},
		Methods: []Method{brew("gitleaks")}},
	{ID: "trivy", Name: "Trivy", Purpose: "Scans dependencies, container images and IaC for vulnerabilities and misconfigurations; writes SBOMs.",
		Docs: "https://trivy.dev/latest/getting-started/installation/", Binary: "trivy", VersionArgs: []string{"--version"}, Timeout: 20 * time.Second,
		Methods: []Method{brew("trivy")}},
	{ID: "semgrep", Name: "Semgrep", Purpose: "Static analysis (SAST) for security bugs in the code.", Docs: "https://semgrep.dev/docs/getting-started/quickstart",
		Binary: "semgrep", VersionArgs: []string{"--version"}, Timeout: 30 * time.Second,
		Methods: []Method{brew("semgrep")}},
	{ID: "osv-scanner", Name: "OSV-Scanner", Purpose: "Checks lockfiles against the OSV vulnerability database.", Docs: "https://google.github.io/osv-scanner/installation/",
		Binary: "osv-scanner", VersionArgs: []string{"--version"},
		Methods: []Method{brew("osv-scanner")}},
	{ID: "govulncheck", Name: "govulncheck", Purpose: "Reports known vulnerabilities that Go code actually calls.", Docs: "https://go.dev/doc/tutorial/govulncheck",
		// -version contacts the vulnerability database, so it fails offline; -h is local and exits 0.
		Binary: "govulncheck", VersionArgs: []string{"-h"},
		Methods: []Method{{Manager: "go", Command: []string{"go", "install", "golang.org/x/vuln/cmd/govulncheck@latest"}, Auto: true}}},
	{ID: "syft", Name: "Syft", Purpose: "Generates a software bill of materials (SBOM) for releases.", Docs: "https://github.com/anchore/syft#installation",
		Binary: "syft", VersionArgs: []string{"version"},
		Methods: []Method{brew("syft")}},
	{ID: "checkov", Name: "Checkov", Purpose: "Scans Terraform, Kubernetes, Helm and Dockerfiles for misconfigurations.", Docs: "https://www.checkov.io/2.Basics/Installing%20Checkov.html",
		Binary: "checkov", VersionArgs: []string{"--version"}, Timeout: 30 * time.Second,
		Methods: []Method{brew("checkov")}},
	{ID: "hadolint", Name: "Hadolint", Purpose: "Lints Dockerfiles against build best practices.", Docs: "https://github.com/hadolint/hadolint#install",
		Binary: "hadolint", VersionArgs: []string{"--version"},
		Methods: []Method{brew("hadolint")}},
	{ID: "promtool", Name: "promtool (Prometheus)", Purpose: "Validates and unit-tests Prometheus alerting and recording rules.", Docs: "https://prometheus.io/docs/prometheus/latest/installation/",
		Binary: "promtool", VersionArgs: []string{"--version"},
		Methods: []Method{withNote(brew("prometheus"), "Installs Prometheus, which ships promtool.")}},
	{ID: "spectral", Name: "Spectral", Purpose: "Lints OpenAPI contracts against API design rules.", Docs: "https://docs.stoplight.io/docs/spectral/b8391e051b7d8-installation",
		Binary: "spectral", VersionArgs: []string{"--version"},
		Methods: []Method{npmGlobal("@stoplight/spectral-cli")}},
	{ID: "k6", Name: "k6", Purpose: "Runs load tests as code with pass/fail thresholds.", Docs: "https://grafana.com/docs/k6/latest/set-up/install-k6/",
		Binary: "k6", VersionArgs: []string{"version"},
		Methods: []Method{brew("k6")}},
}

func withNote(m Method, note string) Method {
	m.Note = note
	return m
}

// Lookup returns the catalog entry for id.
func Lookup(id string) (Tool, bool) {
	for _, t := range Catalog {
		if t.ID == id {
			return t, true
		}
	}
	return Tool{}, false
}
