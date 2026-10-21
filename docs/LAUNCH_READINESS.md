# Launch Readiness — operator guide

> Classification: INTERNAL

Use this guide when an initiative, a new service or a new project has to go to production. The
orchestrator takes it through six readiness steps. Each step is a stage that an agent runs, and each
one stops at a gate that you confirm before the next stage starts:

| # | Step | Stage ID | Built-in skill | Changes code | Tools it needs |
|---|---|---|---|---|---|
| 1 | Check how we release today | `release_audit` | `launch-release-audit` | no (read-only audit) | — |
| 2 | Make releases automatic | `release_automation` | `launch-release-automation` | yes | actionlint, hadolint, docker (all recommended) |
| 3 | Watch the system and send alerts | `observability_alerting` | `launch-observability` | yes | promtool (recommended) |
| 4 | Check security automatically | `security_automation` | `launch-security-scanning` | yes | **gitleaks, trivy, semgrep**; osv-scanner, govulncheck, syft, checkov, docker |
| 5 | Set up the API for customers | `customer_api_setup` | `launch-customer-api` | yes | **spectral**; node |
| 6 | Test with heavy traffic and prepare for problems | `load_resilience` | `launch-load-resilience` | yes | **k6**; docker |

**Bold** tools are *required*. A stage does not start while one of them is missing, unless you
choose to run without it. All other tools are *recommended*: they are reported, but never block a
stage.

---

## 1. Before you start: install the tools

Choose any one of these ways. They all install the same tools.

| Where | How |
|---|---|
| **Tool Hub** (`/tools`) | The **Launch Readiness** group → **Install all N missing**. Or install, update, pin or roll back one package at a time. |
| **CLI** | `go run ./cmd/daemon packages setup launch-readiness` installs every missing package in the group. To install one: `go run ./cmd/daemon packages install trivy`. |
| **When a stage starts** | If a required tool is missing, the run stops with an **Install tools before running …** prompt. Install with one click, or choose **Run without them**. |
| **Workspace & Repos → Toolchain** | Lists every tool the task needs, including the tools of later readiness stages, with one-click install. |

How each tool gets installed:

| Tool | Installed with | Version pinning / rollback |
|---|---|---|
| gitleaks, trivy, semgrep, osv-scanner, syft, checkov, hadolint, actionlint, k6 | Homebrew | No (system package) |
| promtool | Homebrew (`prometheus` formula, which ships promtool) | No |
| spectral | npm (`@stoplight/spectral-cli`), versioned under the orchestrator home | Yes |
| govulncheck | `go install` (`golang.org/x/vuln`), versioned | Yes |
| docker | Docker Desktop, OrbStack or Colima (install it yourself on macOS) | No |

On Linux without Homebrew, the prompt shows the docs link for each tool. The orchestrator only runs
commands without sudo for you. Anything that needs sudo is shown for you to run in your own terminal.

---

## 2. Pick the workflow

On the Mission Control board, open **New Task** and choose a **Workflow**:

| Workflow | ID | Use it for | Stages |
|---|---|---|---|
| **Production Launch Readiness (7-Stage)** | `launch_readiness` | A service that already works and now has to be production-ready | the 6 readiness steps → go/no-go sign-off |
| **New Service / Initiative to Production (14-Stage)** | `new_service_sdlc` | Something new, from the requirement onward | PRD → ATDD → tech doc → plan → implementation → E2E → UAT → the 6 readiness steps → go/no-go sign-off |

Write the task title and description as you would for any task. Include whatever you already know
about the service: the business owner, its users, expected traffic, uptime targets, and
compliance-relevant data such as personal data. The agents treat anything you don't give them as an
**ASSUMPTION** and ask you to confirm it.

**Only some of the steps?** Choose **Partial Stage Slice** and set a start and halt stage. For
example, start at `security_automation` and halt at `security_automation` to run only the security
step on an existing service.

---

## 3. Assign repositories

Every stage except `release_audit` changes code, so assign the service's repositories (and the
infrastructure or deployment repositories, if they are separate) in **Workspace & Repos**. Code
stages edit only inside the task's own git worktree. You review the edits in the **Changes** tab.

---

## 4. Review the routing plan

The routing step of **New Task** (later, the task's **Method & Model** tab) shows one row per stage.
For readiness stages, each row shows:

- **Skills**: the built-in playbook, marked *built in*. It always loads for the stage. You can add up to
  5 more of your own skills, such as an organization security standard, and they are used alongside it.
- **Tools**: each tool with ✓ (installed) or ✗ (missing). Missing required tools are highlighted, with
  a link to the Tool Hub.

Change the method or model per stage if you need to.

---

## 5. Run each stage and confirm its gate

For every stage:

1. **Run** it. The agent gets the stage brief, the playbook skill, and a list of which of the stage's
   tools are installed on this machine.
2. The agent runs the installed tools locally to get a baseline (for example `gitleaks`, `trivy fs`,
   `semgrep`). For a missing tool, it still writes the tool into the CI pipeline and lists it under
   *Operator actions*. **It never installs software itself.**
3. When the stage finishes, the console shows **Confirm before approving:** followed by the gate
   checklist. The agent's Summary ends with the same checklist, each item marked ✓, ✗ or n/a with a
   reason. The task's **Stage plan** also shows the checklist.
4. **Approve** to move to the next stage, or **Reject with feedback** to re-run the stage in the same
   conversation.

### What each gate confirms

**1. Check how we release today**
- Every step of the release path is marked automated, manual or unknown, with the file it comes from
- Each scorecard area has a score and evidence
- Gaps are ranked and each names the readiness stage that closes it
- Unknowns are written as questions the operator can answer

**2. Make releases automatic**
- The pipeline is code in the repository and was validated (linter or dry run named)
- Production promotion reuses the staging artifact and needs an approval
- Rollback is one documented command and migrations are backward compatible
- No secret values in files, and every secret the operator must create is listed

**3. Watch the system and send alerts**
- SLOs are defined for each critical journey (targets marked as assumptions until confirmed)
- Every alert has a severity, a route and a runbook
- Logs redact personal data and secrets, with a test proving it
- Liveness and readiness endpoints exist and are tested

**4. Check security automatically**
- Each check runs in the pipeline and its blocking rule is stated
- Existing findings are triaged, and every suppression has a reason, owner and expiry
- Code review findings name a risk area and file:line
- Personal-data fields are inventoried with their protection

**5. Set up the API for customers**
- The OpenAPI spec covers every public operation and is validated against the implementation in CI
- Auth, tenant isolation, rate limits and idempotency are tested
- Versioning and deprecation policy is written down
- Customer docs let a new client make a first successful call

**6. Test with heavy traffic and prepare for problems**
- Load tests are code with SLO thresholds, and their results (or why they were not run) are reported
- The breaking point and how the service fails there are known
- Resilience gaps are listed with file:line and a fix
- Runbooks, a rollback drill and a capacity plan exist

**Go / No-Go sign-off**: every readiness checklist is approved, or each open item has an owner and a
date, and the rollout and rollback plan is agreed.

### Each stage's output

Each stage's report is saved as `stages/<stage_id>.md` in the task's artifacts (the **Output** button
on the Stage plan). Later stages build on earlier ones. For example, `release_automation` works
through the gaps that `release_audit` found. Code changes go through the normal Changes and pull
request flow.

---

## 6. Safety rules built into the skills

These rules apply to every readiness skill. They follow the organization's data-handling and
ISO/IEC 27001/27701 obligations:

- **Secrets management**: a secret the agent finds is reported as *file and line, masked*, with a
  recommendation to rotate it. The value is never printed.
- **Personal data** (NIK/KTP, KK, phone, email, bank and card numbers) is masked in reports, logs and
  example payloads. Load tests use synthetic data only.
- **Never production**: DAST scans and load tests run only against staging or a performance
  environment, and load tests only after you approve them.
- **No invented facts**: CVE IDs, CVSS scores, legal articles, control numbers and package versions
  come from tool output or a provided source. Otherwise they are marked *unverified*. Numeric targets
  (SLOs, rate limits, response times) are labelled ASSUMPTION until you confirm them.
- **Use what exists**: the agent extends the CI system, monitoring vendor and scanners the
  repositories already use, and does not add a second one.

---

## 7. Customize the skills

At startup, the six playbooks are written to `.sdlc/skills/launch-*/SKILL.md` and listed on the
**Skills** page as project skills.

| You want to | Do this |
|---|---|
| Change a playbook (your org's CI vendor, SLO defaults, security policy) | Edit `.sdlc/skills/<skill>/SKILL.md`. **Your edit is kept:** upgrades only replace files you have not changed (tracked in `.builtin.json`). |
| Go back to the built-in version | Delete the edited file (or the skill folder) and restart the daemon. |
| Turn a playbook off | Disable it on the Skills page. The stage then runs from its brief and any skills you attached. |
| Add your own guidance to a stage | Attach a skill to the stage in the routing plan. It loads alongside the playbook. |
| Use the playbooks outside the orchestrator | Copy a `.sdlc/skills/launch-*` folder into `~/.claude/skills/`. The skills are standard Claude Code `SKILL.md` bundles. |
| Build your own workflow with these stages | **Settings → Workflows**: the six stage IDs are available in the stage builder, and in `.sdlc/workflow.yaml`. Add `gate_criteria` (items separated by `;`) to any stage to get a gate checklist. |

---

## 8. Troubleshooting

| Symptom | Cause / fix |
|---|---|
| "Not started: this stage verifies changes with Trivy (`trivy`)…" | A required tool is missing. Install it from the prompt or the Tool Hub, then run again. Or choose **Run without them**: the agent skips those checks and lists them in its summary. |
| A tool installed from your own terminal still shows ✗ | Click **Recheck** in the toolchain panel or the Tool Hub. The daemon re-reads PATH. |
| The playbook shows as "unavailable" or isn't loaded | It is disabled on the Skills page, or `.sdlc/skills/launch-*` was deleted. Restart the daemon to reinstall it. |
| "… changes code, so it needs repositories" | Assign repositories in Workspace & Repos. |
| Homebrew isn't installed | Install it from https://brew.sh, or use each tool's docs link in the prompt. |

---

## 9. API reference (short)

- `GET /api/v1/workflows` includes `launch_readiness` and `new_service_sdlc`. Readiness stages carry
  `gate_criteria` and `metadata.skill`.
- `POST /api/v1/tasks/analyze` and `GET /api/v1/tasks/{id}/routing` add `playbooks`
  (`{stage_id: skill}`) and `stage_tools` (`{stage_id: [{id, name, why, required, installed}]}`).
- `GET /api/v1/tasks/{id}/toolchain` and `GET /api/v1/toolchain?task={id}` include the current
  stage's tools (required as declared) and the later readiness stages' tools (as recommended).
- `POST /api/v1/packages/setup?category=launch-readiness` installs every missing package in the
  category. Packages in a category are independent, so one failure does not stop the others.
