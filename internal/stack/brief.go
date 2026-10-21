package stack

import (
	"fmt"
	"path"
	"strings"
)

// guide is what an agent should know about a technology: the convention to keep and the commands
// that change shared environments. Verification commands are built in checks.
type guide struct {
	practice string
	never    string
}

var guides = map[string]guide{
	"go": {practice: "Go: gofmt'd code; pass context.Context through calls; wrap errors with %w; table-driven tests next to the code you change."},
	"postgresql": {
		practice: "PostgreSQL: change the schema only through a new migration in the repository's existing migration tool and folder (never edit an applied one); keep it backward compatible with the running code and reversible; parameterised queries only; the connection string comes from each environment's configuration — never hard-code a database host.",
		never:    "migrations, seeds or `psql` writes against a shared (dev/staging/production) database",
	},
	"java": {
		practice: "Java: follow the framework and layering the module already uses (e.g. Spring controller → service → repository); constructor injection; never swallow exceptions; JUnit 5 tests beside the code you change; keep the build's Java version.",
		never:    jvmPublish,
	},
	"kotlin": {
		practice: "Kotlin: null safety without `!!`; `val` and data classes by default; coroutines only in the scopes the code already uses (never GlobalScope); keep ktlint/detekt clean when the build runs them.",
		never:    jvmPublish,
	},
	"android": {
		practice: "Android: follow the app's UI toolkit (Compose or Views) and architecture (ViewModel, DI) as they are; user-facing text in string resources; no secrets or API keys in the app — read them from the build's secret configuration.",
		never:    "signing with release keys or uploading builds (Play Console, `fastlane supply`, gradle-play-publisher, Firebase App Distribution)",
	},
	"dart": {
		practice: "Dart: sound null safety — no `dynamic` or `!` to silence the analyzer; keep analysis_options.yaml clean; never hand-edit generated files (*.g.dart, *.freezed.dart) — regenerate them.",
		never:    "`dart pub publish` / `flutter pub publish`",
	},
	"flutter": {
		practice: "Flutter: small widgets with const constructors; the app's existing state management (Bloc, Provider or Riverpod) and routing; no business logic in build(); user-facing text from the l10n files; widget tests find widgets by Key or semantics label.",
		never:    "store or tester uploads (`fastlane deliver|supply` or other release lanes, TestFlight, Firebase App Distribution)",
	},
	"react":      {practice: "React: function components and hooks, using the state, data-fetching and styling libraries the app already has; accessible markup (labels, roles) so tests can select by role."},
	"typescript": {practice: "TypeScript: no `any`, `@ts-ignore` or non-null assertions to silence the compiler; type API payloads from the OpenAPI spec when the repository generates them."},
	"keycloak": {
		practice: "Keycloak (OIDC): validate tokens server-side (signature via JWKS, issuer, audience, expiry); browser apps use Authorization Code + PKCE; " +
			"machine-to-machine clients use the client-credentials grant — require the expected audience and a scope per operation, map the token's client (`azp`) to its tenant on the server and scope every query to that tenant, never to a tenant id from the request; " +
			"test a missing, expired and wrong-audience token (401), a missing scope (403) and a cross-tenant read; client secrets never go in code, logs or committed realm exports.",
		never: "Keycloak admin API or `kcadm.sh` changes against a shared realm",
	},
	"spicedb": {
		practice: "SpiceDB: decide access with CheckPermission at the API boundary, not with roles copied into the app; a schema change is a migration — keep existing relations working and update the schema's validation tests with it.",
		never:    "`zed schema write` or relationship writes against a live SpiceDB",
	},
	"azure-container-apps": {
		practice: "Azure Container Apps: change apps through the repository's IaC or pipeline, never the portal or CLI; secrets come from Key Vault references or managed identity, not plain environment values; keep health probes and scaling rules.",
		never:    "`az containerapp create|update|up|delete` or any other mutating `az` command",
	},
	"cloudflare": {
		practice: "Cloudflare: DNS, WAF, Workers and Pages changes go through the repository's IaC or wrangler config; secrets go through CI or `wrangler secret`, never into wrangler config.",
		never:    "`wrangler deploy|publish|delete` or `wrangler secret put`",
	},
	"opentofu": {
		practice: "OpenTofu: change modules and inputs, never state or generated files; pin provider and module versions; no secrets in .tf/.tfvars — reference the secret store.",
		never:    "`tofu apply|destroy|import|state …|force-unlock`; `tofu plan` too, unless the operator approves (it reads real state with cloud credentials)",
	},
	"terragrunt": {
		practice: "Terragrunt: keep shared settings in the root/parent config and units DRY through include; change an environment's unit only for the environment the task names.",
		never:    "`terragrunt apply|destroy`, any run-all/`--all` apply, or `terragrunt plan` unless the operator approves",
	},
	"github-actions": {
		practice: "GitHub Actions: least-privilege `permissions:`, third-party actions pinned to a commit SHA, cloud access through OIDC federation instead of long-lived secrets; never print secrets.",
		never:    "`gh workflow run`, or pushing a tag or a deploy/release branch that triggers a deployment (task branches and pull requests are fine)",
	},
	"docker": {
		practice: "Containers: multi-stage builds, a pinned base image, a non-root USER, nothing secret in layers or build args.",
		never:    "`docker push` to a shared registry",
	},
	"playwright":    {practice: "Playwright: role and label locators (getByRole, getByLabel), web-first assertions, no fixed waits; each test creates the data it needs."},
	"opentelemetry": {practice: "OpenTelemetry: use the existing tracer and meter providers, semantic-convention names and context propagation; never put personal data, tokens or unbounded values (ids, emails) in span attributes, metric labels or logs."},
	"grafana":       {practice: "Grafana: dashboards and alerts as code; keep each dashboard's uid stable and use datasource variables instead of hard-coded datasource uids."},
	"nginx": {
		practice: "Nginx gateway: keep every route's rate limit (`limit_req`, or the Lua limiter) and the key it limits by (e.g. the OAuth client_id), and keep the access log's fields and format — dashboards, alerts and audits read them; " +
			"never log Authorization headers, tokens, client secrets or request bodies; a new route gets an explicit `location` with timeouts and a body-size limit, and internal endpoints (metrics, admin) stay unrouted.",
		never: "`nginx -s reload|stop` or config pushes against a shared gateway",
	},
	"openresty": {practice: "OpenResty (Lua): `local` everything; only cosocket-based I/O in request phases (no blocking libraries); keep `lua_shared_dict` names and limiter keys stable; treat a token-check error as a rejection, never as a pass."},
	"openapi":   {practice: "OpenAPI: the spec is the contract — change it together with the handlers and regenerate clients/servers with the repository's generator; additive changes only unless the API is versioned; document error responses."},
}

const jvmPublish = "`mvn deploy`, `gradle publish` or any task that uploads artifacts to a shared repository"

// check is one verification command and the repository-relative folder it runs in.
type check struct{ dir, cmd string }

// checks lists the commands that verify a change in this repository, in stack order.
func (p Profile) checks() []check {
	var out []check
	add := func(dir, cmd string) {
		for _, c := range out {
			if c.dir == dir && c.cmd == cmd {
				return
			}
		}
		out = append(out, check{dir, cmd})
	}
	if t, ok := p.Get("go"); ok {
		for _, d := range t.Dirs {
			add(d, "go build ./... && go vet ./... && go test ./...")
			if p.lints[d] || p.lints["."] {
				add(d, "golangci-lint run (if installed)")
			}
		}
	}
	for _, c := range p.nodeChecks() {
		add(c.dir, c.cmd)
	}
	if p.Has("java") || p.Has("kotlin") || p.Has("android") {
		for _, b := range p.jvmBuilds {
			switch {
			case b.gradle:
				add(b.dir, wrapped(b.wrapper, "./gradlew", "gradle")+" check — narrow with `--tests <Class>` while iterating")
			default:
				add(b.dir, wrapped(b.wrapper, "./mvnw", "mvn")+" -q verify — narrow with `-Dtest=<Class> test` while iterating")
			}
		}
	}
	for _, d := range p.dartPkgs {
		tool := "dart"
		if d.flutter {
			tool = "flutter"
		}
		if d.buildRunner {
			add(d.dir, "dart run build_runner build --delete-conflicting-outputs — after changing annotated models")
		}
		add(d.dir, "dart format --output=none --set-exit-if-changed .")
		add(d.dir, tool+" analyze")
		if d.tests {
			add(d.dir, tool+" test")
		}
	}
	if t, ok := p.Get("opentofu"); ok {
		for _, d := range topDirs(t.Dirs) {
			add(d, "tofu fmt -check -recursive")
		}
		add(".", "tofu init -backend=false -input=false && tofu validate — in each module you change")
	}
	if t, ok := p.Get("terragrunt"); ok {
		for _, d := range topDirs(t.Dirs) {
			add(d, "terragrunt hclfmt --check (newer releases: terragrunt hcl fmt --check)")
		}
	}
	if t, ok := p.Get("spicedb"); ok {
		for _, f := range t.Files {
			if strings.HasSuffix(f, ".zed") {
				add(".", "zed validate "+f+" (if zed is installed)")
			}
		}
	}
	if t, ok := p.Get("cloudflare"); ok {
		for _, f := range t.Files {
			if strings.HasPrefix(path.Base(f), "wrangler.") {
				add(dirOf(f), "npx wrangler deploy --dry-run")
			}
		}
	}
	if t, ok := p.Get("openapi"); ok {
		for _, f := range t.Files {
			add(".", "npx @redocly/cli lint "+f)
		}
	}
	if p.Has("nginx") || p.Has("openresty") {
		bin := "nginx"
		if p.Has("openresty") {
			bin = "openresty" // plain nginx rejects the *_by_lua directives
		}
		tested := false
		for _, id := range []string{"nginx", "openresty"} {
			t, _ := p.Get(id)
			for _, f := range t.Files {
				if ok, _ := path.Match("nginx*.conf", path.Base(f)); ok {
					tested = true
					add(dirOf(f), bin+` -t -p "$PWD" -c "$PWD/`+path.Base(f)+`" — upstream names must resolve; when they are compose services, run it in the gateway container`)
				}
			}
		}
		if !tested {
			add(".", bin+" -t — inside the gateway container, after changing its config")
		}
		if p.Has("openresty") {
			for _, d := range topDirs(p.luaDirs) {
				add(d, "luacheck . (if installed)")
			}
		}
	}
	for _, f := range p.composes {
		add(dirOf(f), "docker compose -f "+path.Base(f)+" up -d --wait — only before e2e tests or evidence, then call each service's health endpoint (e.g. /healthz); `docker compose down` afterwards, without -v")
	}
	if p.Has("github-actions") {
		add(".", "actionlint (if installed)")
	}
	return out
}

// nodeChecks runs the package's own scripts where it has them — they know the project's flags —
// and falls back to the tools themselves.
func (p Profile) nodeChecks() []check {
	var out []check
	techDirs := func(id string) map[string]bool {
		m := map[string]bool{}
		if t, ok := p.Get(id); ok {
			for _, d := range t.Dirs {
				m[d] = true
			}
		}
		return m
	}
	ts, react, pw := techDirs("typescript"), techDirs("react"), techDirs("playwright")
	for _, dir := range sortedKeys(p.nodePkgs) {
		if !ts[dir] && !react[dir] && !pw[dir] {
			continue
		}
		pkg := p.nodePkgs[dir]
		run := func(script string) string { return pkg.manager + " run " + script }
		if s := pkg.first("typecheck", "type-check", "check-types", "tsc"); s != "" {
			out = append(out, check{dir, run(s)})
		} else if ts[dir] {
			out = append(out, check{dir, pkg.exec() + " tsc --noEmit"})
		}
		if s := pkg.first("lint"); s != "" {
			out = append(out, check{dir, run(s)})
		}
		if s := pkg.first("test:unit", "test"); s != "" {
			out = append(out, check{dir, "CI=1 " + run(s)}) // CI=1 stops watch mode in vitest/jest
		}
		if s := pkg.first("build"); s != "" && react[dir] {
			out = append(out, check{dir, run(s)})
		}
		if pw[dir] {
			if s := pkg.first("test:e2e", "e2e", "playwright"); s != "" {
				out = append(out, check{dir, run(s) + " — only the specs your change touches, with the app reachable"})
			} else {
				out = append(out, check{dir, pkg.exec() + " playwright test — only the specs your change touches, with the app reachable"})
			}
		}
	}
	return out
}

func wrapped(wrapper bool, script, tool string) string {
	if wrapper {
		return script
	}
	return tool
}

func (n nodePkg) first(names ...string) string {
	for _, s := range names {
		if n.scripts[s] {
			return s
		}
	}
	return ""
}

func (n nodePkg) exec() string {
	switch n.manager {
	case "pnpm":
		return "pnpm exec"
	case "yarn":
		return "yarn"
	case "bun":
		return "bunx"
	}
	return "npx"
}

// Repo is a named repository and its detected stack.
type Repo struct {
	Name    string
	Profile Profile
}

// Brief tells an agent the stack of the repositories it works in: what each is built with, how to
// verify a change, the conventions to keep and what it must never run. It is empty when nothing
// was detected.
func Brief(repos []Repo) string {
	var stacks, verify []string
	practices, nevers := map[string]bool{}, map[string]bool{}
	var practiceList, neverList []string
	for _, r := range repos {
		if len(r.Profile.Techs) == 0 {
			continue
		}
		var names []string
		for _, t := range r.Profile.Techs {
			name := t.Name
			if !t.AtRoot() && len(t.Dirs) > 0 {
				name += " (" + strings.Join(t.Dirs, ", ") + ")"
			}
			names = append(names, name)
			g := guides[t.ID]
			if g.practice != "" && !practices[t.ID] {
				practices[t.ID] = true
				practiceList = append(practiceList, g.practice)
			}
			if g.never != "" && !nevers[g.never] { // Java and Kotlin share one
				nevers[g.never] = true
				neverList = append(neverList, g.never)
			}
		}
		stacks = append(stacks, fmt.Sprintf("- %s: %s", r.Name, strings.Join(names, ", ")))
		for _, c := range r.Profile.checks() {
			where := r.Name
			if c.dir != "." {
				where += "/" + c.dir
			}
			cmd, note := splitNote(c.cmd)
			verify = append(verify, fmt.Sprintf("- %s: `%s`%s", where, cmd, note))
		}
	}
	if len(stacks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Tech stack\nDetected from the assigned repositories' files (folders are relative to each repository):\n")
	b.WriteString(strings.Join(stacks, "\n"))
	if len(verify) > 0 {
		b.WriteString("\n\nVerify — run the narrowest check that covers your change first, the rest before you report it done; " +
			"skip a tool that is not installed and say so instead of installing it:\n")
		b.WriteString(strings.Join(verify, "\n"))
	}
	if len(practiceList) > 0 {
		b.WriteString("\n\nConventions:\n- ")
		b.WriteString(strings.Join(practiceList, "\n- "))
	}
	if len(neverList) > 0 {
		b.WriteString("\n\nNever run these — they change shared environments; describe the command for the operator instead:\n- ")
		b.WriteString(strings.Join(neverList, "\n- "))
	}
	b.WriteString("\n")
	b.WriteString(environmentBrief(repos))
	return b.String()
}

// splitNote separates a check's explanatory tail (from " — " or " (") so only the command is
// rendered as code.
func splitNote(cmd string) (string, string) {
	i := -1
	for _, sep := range []string{" — ", " ("} {
		if j := strings.Index(cmd, sep); j > 0 && (i < 0 || j < i) {
			i = j
		}
	}
	if i < 0 {
		return cmd, ""
	}
	return cmd[:i], cmd[i:]
}

// topDirs reduces folders to their top-level folder, so formatters run once per tree.
func topDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		top := strings.SplitN(d, "/", 2)[0]
		addUnique(&out, top, maxDirs)
	}
	if len(out) > 1 {
		for _, d := range out {
			if d == "." {
				return []string{"."}
			}
		}
	}
	return out
}
