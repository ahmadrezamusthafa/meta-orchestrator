---
name: launch-release-audit
description: "Launch readiness step 1 — check how the service is released today. Maps the real path from commit to production (branching, build, tests, artifacts, environments, deploy, rollback, approvals, secrets, database migrations) from the repository and its CI/CD config, scores each area, and produces the gap list the later readiness steps close. Read-only. Built into the orchestrator."
---

# launch-release-audit — how do we release today?

Every later readiness step (automatic releases, monitoring, security checks, the customer API,
load tests) builds on this audit. Get the facts right: what the repository **actually does**, not
what a README says it should do. Change nothing — this step is read-only.

## 1. Collect the evidence

Look in every assigned repository, and note the path of each file you base a finding on:

- **CI/CD config**: `.github/workflows/`, `.gitlab-ci.yml`, `bitbucket-pipelines.yml`, `Jenkinsfile`,
  `.circleci/`, `azure-pipelines.yml`, `buildkite/`, Argo CD / Flux manifests, `Makefile` release targets.
- **Build and packaging**: `Dockerfile`s, `docker-compose*.yml`, Helm charts, Kustomize, Terraform,
  Serverless / SAM / CDK, Procfile, buildpacks.
- **Versioning**: tags in `git tag --sort=-creatordate | head -20`, `CHANGELOG*`, release-please /
  semantic-release / goreleaser config, version constants.
- **History**: `git log --merges --oneline -30`, branch names (`git branch -a`), release branches,
  hotfix patterns, how often `main`/`master` changes.
- **Environments and config**: `.env.example`, `config/*.yml`, per-environment values, feature-flag
  libraries (Flipper, Flagsmith, LaunchDarkly, Unleash, …).
- **Database changes**: migration folders and how they run (on boot, a CI job, by hand).
- **Docs**: `README`, `RUNBOOK*`, `docs/`, `CODEOWNERS`, PR templates.

Do not guess what you cannot see. Where something happens outside the repository (a deploy
button, a manual step on a server, a ticket to another team), record it as **Unknown — ask: …**
with the exact question the operator should answer.

## 2. Map the release path

Write the path as ordered steps, from a merged change to running in production, and mark each:
`automated`, `manual`, or `unknown`. Include who or what triggers each step and the approval it needs.

## 3. Score each area (0–3)

| Area | 0 | 1 | 2 | 3 |
|---|---|---|---|---|
| Build | manual / laptop | scripted | CI on every PR | CI, reproducible, cached, pinned |
| Tests in the pipeline | none | some, not blocking | blocking unit + integration | blocking, incl. contract/e2e on main |
| Artifact | built per env | built once, untagged | immutable, versioned | immutable, signed, with SBOM |
| Deploy | manual on server | scripted by hand | automated to staging | automated to prod with approval gate |
| Rollback | not possible / undocumented | manual, documented | one command / redeploy previous | automatic on failed health check |
| DB migrations | by hand | scripted, separate | in pipeline | in pipeline, backward compatible, tested |
| Secrets | in repo / env files | CI variables | secret manager | secret manager, rotated, least privilege |
| Release safety | big-bang | feature flags exist | flags or canary | progressive delivery with automated checks |
| Traceability | none | tags | changelog + tags | every release linked to PRs/tickets |

## 4. Flag the risks

Flag immediately (top of the report):

- Secrets, keys or credentials committed to the repository: give the **file path and line only**,
  never the value, and recommend rotation.
- Personal data (KTP/NIK, phone, email, bank numbers) in fixtures, seeds or logs: mask it.
- A production deploy that anyone can run without review.
- No way to roll back.

## Tools

None beyond `git` and read access to the repositories. This step only reads.

## Output

Use exactly these headings:

```
## Release path today
(ordered steps, each: automated | manual | unknown — trigger — approval — source file)

## Scorecard
(the table above, filled with the score and one line of evidence per area)

## Gaps
(numbered, most severe first; each: what is missing, why it matters, which later step fixes it:
release_automation | observability_alerting | security_automation | customer_api_setup | load_resilience)

## Unknowns — questions for the operator

## Follow-up tasks
(the `followups` JSON block from the stage brief: remaining gaps grouped into a few tasks)
## Summary
```
