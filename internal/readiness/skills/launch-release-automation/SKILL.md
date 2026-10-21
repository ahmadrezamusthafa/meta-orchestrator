---
name: launch-release-automation
description: "Launch readiness step 2 — make releases automatic. Builds or hardens the CI/CD pipeline as code in the repository: build once into an immutable versioned artifact, blocking tests, automated deploy to staging, gated promotion to production, database migrations, smoke checks, and a one-command rollback. Fixes the gaps from the release audit. Built into the orchestrator."
---

# launch-release-automation — make releases automatic

Goal: a merged change reaches production through a pipeline that is **repeatable, reviewable and
reversible**, with a human approval only where the business wants one. Start from the release
audit's gaps (earlier in this conversation, or the task's `stages/release_audit.md`). Extend the CI
system the repository already uses — never introduce a second one.

## Principles

1. **Pipeline as code**, in the repository, reviewed like any change.
2. **Build once, promote the same artifact**: one image or package per commit, tagged with the
   commit SHA (plus a semantic version on release). Environments differ by config, never by build.
3. **Every gate is automated except the business approval**: lint, unit, integration and contract
   tests block the merge; production promotion needs an approval (a protected environment / manual
   job) and a green staging deploy.
4. **Reversible**: a documented one-command rollback to the previous artifact; database migrations
   are backward compatible (expand → migrate → contract) so rolling back the code is safe.
5. **Least privilege, no long-lived secrets**: deploy credentials come from the CI system's secret
   store or OIDC federation to the cloud; nothing secret in the repository or in logs.
6. **Fast feedback**: cache dependencies, run independent jobs in parallel, target < 10 minutes to a
   deployable artifact. ASSUMPTION unless the operator gives a different target.

## Build the pipeline

Stages, in order — keep what already exists and works, add what is missing:

1. **Verify** (every PR): install with the lockfile, lint/format check, unit tests, build.
2. **Integration** (every PR): integration and contract tests against containers (database, cache,
   queue) started by the pipeline.
3. **Package** (main): build the artifact once, tag `sha-<short>` and on tags `vX.Y.Z`; push to the
   registry the organization uses.
4. **Deploy staging** (main, automatic): run migrations, deploy, wait for readiness, run smoke
   checks against the health endpoint and one critical path.
5. **Promote production** (approval): same artifact, migrations, deploy (rolling, canary or
   blue/green — whatever the platform supports), smoke checks, automatic rollback or a clear
   failure if they fail.
6. **Release notes**: changelog from conventional commits or PR titles; link the release to its tickets.

Also add, where missing:

- `concurrency` / locking so two deploys to one environment never overlap.
- Branch protection expectations written in the PR description (required checks, reviewers) — the
  agent cannot change repository settings; list them for the operator.
- A `RELEASING.md` (or a section in `README`) with: how to release, how to roll back, how to run a
  migration, who approves.
- Feature-flag guidance for risky changes so deploy and release are separate.

## Guardrails

- Do not deploy anything from this stage. Validate pipeline files with the CI system's linter or a
  dry run where one exists (`actionlint`, `gitlab-ci-lint`, `bitbucket-pipelines` validator, `helm
  template`, `terraform validate`) and say which you ran.
- Reference secrets by name only (`${{ secrets.DEPLOY_ROLE_ARN }}`, `$REGISTRY_TOKEN`), and list the
  secrets the operator must create.
- Pin third-party CI actions/images to a version or digest the repository already uses; if you add
  a new one, mark its version **unverified** for the operator to confirm.

## Tools

| Tool | Need | Used for |
|---|---|---|
| `actionlint` | recommended | validate GitHub Actions workflows |
| `hadolint` | recommended | lint the Dockerfile the pipeline builds |
| `docker` | recommended | build the image locally to prove the Dockerfile works |

Use the validator of the CI system the repository actually uses (for GitLab, its CI lint API; for Helm, `helm template`; for Terraform/OpenTofu, `validate`). Install from the orchestrator's Tool Hub (**Launch Readiness** → *Install all missing*), the install prompt shown when the stage starts, or `go run ./cmd/daemon packages setup launch-readiness`. Never install software yourself from inside a stage: if a tool is missing, still wire it into the pipeline, give the exact command, and list it under operator actions.

## Output

```
## Pipeline
(stages, triggers, gates — and the file that implements each)

## Changes
(per file: what and why)

## Operator actions
(secrets/variables to create, environments and approvers to configure, branch protection)

## Rollback
(the exact command or button, and how migrations stay safe)

## Gap status
(each gap from the release audit: closed | partly | deferred — why)

## Follow-up tasks
(the `followups` JSON block from the stage brief: remaining gaps grouped into a few tasks)
## Summary
```
