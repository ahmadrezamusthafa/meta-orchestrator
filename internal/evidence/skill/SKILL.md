---
name: test-evidence-video
description: "Test a finished change completely and record the proof: run every affected test suite for real, then record one narrated video per test case — a title card (Given / Expected / steps / commits under test), the test commands with their real output, counts and exit code, API calls with request, response and checks, browser walkthroughs of the running app with each action shown before and after and each check marked Verified, and a result card — plus an annotated screenshot per step and evidence.json. Built into the orchestrator; the default evidence skill of the Implementation stage."
---

# test-evidence-video — test it completely, prove it step by step

Evidence is for a reviewer who did not write the change: they must be able to see, step by step,
what was done and what proves the expected result — without reading the code. It is only worth
something if it is real: commands run by the recorder (it captures the real exit code and
output), API calls and screens of the change actually running. Never hand-write a result, never
fake a passing run, never record against production.

## What good evidence looks like

- **One case per acceptance test case**, named by its id, plus one `SUITE-<repo>` case per
  repository you changed. Cover the negative and edge cases the acceptance criteria name
  (validation errors, empty states, permissions), not only the happy path.
- **Given / When / Then.** `given` states the starting data or state, the steps are the When, and
  `expected` the Then. A reviewer reads the title card and knows what to look for.
- **Every action is followed by a check.** After a `click`, `fill` or `select` that changes the
  screen, add an `expect_*` step for the result the case is about (the row that appears, the
  message, the new URL, the count). Checks are what turn a screen recording into proof: the
  recorder fails the case when one does not hold.
- **Captions say what happens, notes say why it matters.** `caption`: "Choose Paid in the Status
  filter". `note` (optional): "The list is filtered server side — the API call below shows it".
  Write them for a product owner, not a developer.
- **Point at the proof.** Finish a browser case with `annotate` on the element that proves it
  ("Status: PAID", the total, the new badge), labelled.
- **Show every layer the change touches.** A change with any UI gets a browser walkthrough of the
  screen it changes. A backend change gets `api` steps against the service running locally (the
  status and the fields that prove the case). Both get a `run` of the narrowest test that covers it.
- **Show the state before the change acts**, when it matters: e.g. the list with all rows
  (`expect_count`) before the filter, then the filtered list.

## Workflow

1. **Find the test commands.** For every repository you changed, find how its tests really run:
   `Makefile`, `package.json` scripts, `go.mod`, `Gemfile`/`bin/rspec`, CI config
   (`.github/workflows`, `bitbucket-pipelines.yml`, `.gitlab-ci.yml`). Prefer the CI command.
2. **Run them completely first** (outside the recorder) — unit, integration and the specs or e2e
   tests that cover the change. Fix what your change broke and run again until green. A failure
   that also happens without your change (check the base branch or the CI history) is
   pre-existing: keep it, and say so in the case `notes` and the Summary.
3. **Start what the cases need**, from the task worktree, the way the repository documents it (dev
   server, `docker compose`), against a local or test database seeded with fake data. When the app
   or service cannot start locally, keep the `run` parts and add a `card` that says why the browser
   or API part is missing — never skip silently.
4. **Write the storyboard** to `<evidence dir>/storyboard.json` (format below).
5. **Record one case per command**, so each call stays well inside the command time limit:
   ```bash
   node <skill dir>/scripts/record-evidence.mjs <evidence dir>/storyboard.json --case <id>
   ```
   Give the command a timeout of 600000 ms. It prints `✓`/`✗` per case, the steps that failed and
   the browser's console errors, and writes `<id>.mp4` (`.webm` without ffmpeg), `<id>.png` (the
   result card), `shots/<id>-NN.png` (step NN, annotated), `logs/` and merges the case into
   `<evidence dir>/evidence.json`. Exit code 1 means a case did not pass.
6. **Review every take.** Open `<id>.png` and the `shots/` of the case with the Read tool. Each step
   must show what its caption says, the outlined target must be the right element, and nothing
   personal may be readable. A step that failed because of the storyboard (wrong selector, page
   still loading — add a `wait`) → fix the storyboard and re-record the case. A step that failed
   because of the code → fix the code, re-run the tests, re-record. Browser console errors listed
   for a case are worth a look: mention any that the change causes. Expect 1–3 takes.
7. **Clean up**: stop every server you started. Evidence files stay in the evidence dir only —
   never add them to the repository.
8. **Report** a `## Test evidence` section: a table of case id, title, status, steps passed and
   video file; the full-suite results per repository (counts); and any pre-existing failure,
   skipped browser or API part, or console error the change causes.

## Storyboard

```json
{
  "title": "TASK-12 · Filter proforma invoices by status",
  "label": "TASK-12 · Implementation evidence",
  "base_url": "http://localhost:5173",
  "api_base": "http://localhost:8080",
  "repos": ["/abs/worktree/billing", "/abs/worktree/billing-frontend"],
  "cases": [
    {"id": "SUITE-billing-frontend", "title": "Full unit test suite of billing-frontend",
     "steps": [{"action": "run", "command": "npm test -- --reporter=dot", "cwd": "/abs/worktree/billing-frontend",
                "caption": "All unit tests", "suite": "billing-frontend", "timeout_s": 540}]},
    {"id": "ABC-TC-001", "title": "Finance filters the list by Paid", "covers": ["ABC-TC-001"],
     "given": "Three proforma invoices exist: two Paid, one Unpaid",
     "expected": "Only the two paid invoices are listed",
     "steps": [
       {"action": "run", "command": "npx vitest run src/views/proforma -t 'status filter'", "cwd": "/abs/worktree/billing-frontend",
        "caption": "Spec for the status filter", "highlight": "status filter"},
       {"action": "api", "method": "GET", "url": "/api/v1/proforma-invoices?status=paid", "headers": {"Authorization": "Bearer ${TEST_API_TOKEN}"},
        "caption": "The API returns only paid invoices", "expect_status": 200, "expect_body": ["\"paid\""], "expect_no_body": ["\"unpaid\""],
        "note": "The filter is applied server side"},
       {"action": "goto", "target": "/billing/proforma-invoices", "caption": "Open Proforma Invoices"},
       {"action": "expect_count", "target": "[data-testid=invoice-row]", "value": "3", "caption": "All three invoices before filtering"},
       {"action": "select", "target": "label=Status", "value": "Paid", "caption": "Choose Paid in the Status filter"},
       {"action": "expect_count", "target": "[data-testid=invoice-row]", "value": "2", "caption": "Two invoices remain"},
       {"action": "expect_no_text", "value": "Unpaid", "caption": "No unpaid invoice is listed"},
       {"action": "annotate", "targets": [{"target": "text=INV-001", "label": "Paid"}, {"target": "text=INV-003", "label": "Paid"}],
        "caption": "Both remaining invoices are paid"}
     ]}
  ]
}
```

Steps, played in order (each gets its own screenshot and its time in the video):

| action | fields | shows |
|---|---|---|
| `run` | `command`, `cwd` (absolute), `highlight` (regex), `suite` (repository name, for a full-suite run), `timeout_s` (default 480), `live` | runs it **before** the video (or in place with `live: true`, e.g. a seed between browser steps); terminal card with the lines matching `highlight`, the output tail, real exit code, test counts and duration; full log in `logs/`. `suite` makes it the suite result of that repository in evidence.json, with real counts |
| `api` | `method`, `url` (path on `api_base`, or a full local URL), `headers`, `body` (object or string), `expect_status` (number or list; default: not an error), `expect_body` / `expect_no_body` (strings) | request and response side by side, the expected strings highlighted, a ✓/✗ chip per check; the exchange in `logs/` |
| `log` | `file` (relative to the evidence dir), `status`, `command` | a saved log — only for a run the recorder cannot repeat |
| `card` | `caption`, `body`, `kind` | a note, e.g. why a browser part is missing |
| `goto` | `target` (path on `base_url`, or a full local URL) | the page |
| `click` `dblclick` `hover` `check` `uncheck` | `target` | **Action**: target outlined, cursor on it; then **Done**: the screen after |
| `fill` | `target`, `value` (typed key by key; a `${NAME}` value or `secret: true` is filled at once and blurred) | as above |
| `select` | `target` (a native `<select>`), `value` (label or value) | as above; for a custom dropdown, `click` it and `click` the option |
| `upload` | `target` (a file input), `value` (file path) | as above |
| `press` | `value` (key, default Enter) | as above |
| `scroll` `wait` | `target`, or `value` (pixels / milliseconds) | the screen once it is there |
| `expect_text` | `value`, `target` (optional: look only inside it), `label` | **✓ Verified**, the text outlined |
| `expect_no_text` | `value` | ✓ Verified when it is not on the screen |
| `expect_visible` `expect_hidden` | `target`, `label` | ✓ Verified, the element outlined |
| `expect_url` | `value` (part of the URL) | ✓ Verified |
| `expect_count` | `target`, `value` (exact number) | ✓ Verified, every match outlined |
| `annotate` | `targets: [{target, label}]` (or `target` + `label`) | **Look here**: each outlined, numbered and labelled |

Every step takes `caption`, `note` and `hold_ms` (how long it stays on screen).
Targets: `label=…`, `placeholder=…`, `testid=…`, `text=…`, `role=button[name="Save"]`, or CSS.
`${NAME}` in a target, value, URL, header or body is read from the environment — use it for any
login or token, never write a credential into the storyboard; on screen the placeholder is shown,
not the value. A case's `base_url`, `api_base` or `storage_state` overrides the storyboard's.
`repos` (absolute paths) adds repositories to the "commits under test" line; the `cwd` of `run`
steps is included automatically.

After a browser step fails, the case's later browser steps are listed as not run (the screen is no
longer where they expect it); `run` and `api` steps still play. Set `"continue_on_failure": true`
on a case whose browser steps are independent.

## Rules

- Fake data only: test databases, factories, seeds. Never a production or shared environment.
- The recorder blurs email addresses, long numbers and phone numbers on screen and redacts
  secrets, tokens, emails and long numbers in logs, API responses and captions; still check every
  screenshot, and change fixture data that looks real.
- Never print environment variables or `.env` files, and never put credentials in commands.
- One case per recorder call; keep `run` parts to the narrowest command that proves the case,
  except the `SUITE` cases.
