---
name: test-evidence-video
description: "Prove a change works end to end, as specified: find every flow the change touches (screens, APIs, scheduled jobs, workers, consumers, commands, alerts), then record each running for real as a narrated screen recording — browser walkthroughs, API calls, live terminals, and background processes started, triggered and their logs watched — with every action and check annotated, plus a presentation video, a screenshot per step and the spec coverage. Unit tests are regression only. Built into the orchestrator; the default evidence skill of the Implementation stage."
---

# test-evidence-video — prove the feature works, end to end, as specified

Evidence is for a reviewer — a product owner, QA, the client — who did not write the change and
reads no code. It must show them that **every flow the change touches works end to end, as
specified**, in the running system, the way it runs for its users. The **diff** tells you which
flows: every entry point it changes — a screen, an API endpoint, a scheduled job or scheduler, a
worker or queue consumer, a CLI command, a notification or alert. The **spec** tells you what each
must do. And the evidence must **show the system working**: the UI, the API's response, the
terminal of a command, the live log of a job or worker. A unit test passing proves none of that —
unit and integration runs are regression only, and the recorder refuses a flow or acceptance case
made only of them. Never hand-write a result, never fake a passing run, never record against
production.

### A change with no screen

Prove a job, scheduler, worker or consumer the way it runs: `start` it (and what it talks to — a
mock receiver for its alerts or webhooks, a local queue), trigger it (its schedule with the clock
set by an environment variable or a time-travel helper, a rake/CLI task, a queued message, an API
call), and `watch` its live log for what must happen; then `expect_log` for what must happen
exactly so many times or never ("alert sent" exactly once, "retry" never), and check the outcome
(the receiver's record, the database through a `run` query, the API). Use `monitor` to keep a
process's live log in a drawer beside the app while browser steps run.

## What to record

Three kinds of case (`"kind"` in the storyboard):

1. **`flow` — end-to-end user journeys** (`FLOW-1`, `FLOW-2`, …). Each main journey of the
   feature in one continuous video: from where the user starts (sign in, the menu) through every
   screen of the journey to the final outcome — and what the other side shows of it (the record in
   the admin screen, the API's response, the notification). This is the demo that the feature
   works completely; most features have 1–3. Take the journeys from the PRD / user stories.
2. **`acceptance` — one case per acceptance test case** (its ATDD id), or per acceptance criterion
   (`AC-1`, … listed in `criteria`) when the task has no ATDD cases. Preconditions become `given`,
   the steps are driven through the running system, and every expected result gets a check.
   Include the negative and edge cases: validation errors, empty states, permissions, limits.
   A WEB case is a browser walkthrough; an API case is `api` steps against the running service, plus
   the screen that shows the result when there is one.
3. **`suite` — regression** (`SUITE-<repo>`): the full test suite of each repository you changed.

Unit and integration test runs are regression only. They never prove an acceptance case on their
own: an acceptance or flow case shows the system doing it. An existing end-to-end or system test
that drives the real app (Playwright, Cypress, a Capybara feature spec, an API contract test) may
run inside a case in addition to the walkthrough — never instead of it.

Every acceptance case must be covered (`covers` of a flow may list the cases it walks through, but
each acceptance case still gets its own case). The Evidence tab shows the coverage of the spec and
flags any case without evidence.

## How it looks: a narrated screen recording, then a presentation

Each case video is a screen recording of the running app with annotations, step by step: the
pointer glides to each target and clicks with a ripple, small fields are zoomed in while they are
filled, every screen is named in the banner (and flagged when it is new), and every check ends in
a ✓ Verified banner with the proof outlined. The **presentation** (`SUMMARY.mp4`, made with
`--summary` after the cases) then presents the whole evidence to the reviewer: the verdict, the
spec coverage, a chapter per case with its key annotated steps, and the results.

## What good evidence looks like

- **Given / When / Then.** `given` states the starting data or state, the steps are the When, and
  `expected` the Then — in the spec's words. A reviewer reads the title card and knows what to
  look for.
- **Every action is followed by a check.** After a `click`, `fill` or `select` that changes the
  screen, add an `expect_*` step for the result the case is about (the row that appears, the
  message, the new URL, the count). Checks turn a screen recording into proof: the recorder fails
  the case when one does not hold.
- **Captions say what happens, notes say why it matters**, for a product owner, not a developer:
  caption "Choose Paid in the Status filter", note "Only paid invoices are listed — rule FR-2".
- **Introduce each screen.** On the first step that reaches a screen, add `explain` — its regions
  the case is about, labelled in the user's words ("Status filter (new)", "Invoice list") — and
  `screen`, its name. The viewer learns the screen before anything happens on it.
- **Point at the proof.** End a case with `annotate` on what proves it (the status, the total, the
  new badge), labelled in the spec's terms.
- **Show the state before the action**, when it matters: the list with all rows (`expect_count`)
  before the filter, the balance before the refund.
- **Honest about gaps.** When part of a case cannot run locally (a third-party service, data you
  cannot create), record what can run, add a `card` saying exactly what is missing and why, and set
  `"blocked"` on the case to that reason — the case is then reported Blocked, never Passed.

## Workflow

1. **Read the spec**: the PRD, the user stories and acceptance criteria, the ATDD cases, the UI
   spec or designs, the API contract, the tech doc. List the user journeys and the acceptance
   cases to prove. Then read the change, only to know how to run it.
2. **Run the full test suites** of every repository you changed (outside the recorder) — find how
   they really run: `Makefile`, `package.json`, `go.mod`, `Gemfile`, CI config. Fix what your change
   broke and re-run until green. A failure that also happens without your change is pre-existing:
   keep it and say so.
3. **Start the system** from the task worktrees, the way the repositories document it (dev server,
   `docker compose`), every application the journeys pass through, against a local or test
   database seeded with fake data that matches the cases' preconditions. Sign in with a test user
   (credentials from the environment, `${NAME}`).
4. **Write the storyboard** to `<evidence dir>/storyboard.json` (format below): the flows first,
   then the acceptance cases in the spec's order, then the suites; `criteria` when there are no
   ATDD cases.
5. **Record one case per command**, so each call stays well inside the command time limit:
   ```bash
   node <skill dir>/scripts/record-evidence.mjs <evidence dir>/storyboard.json --case <id>
   ```
   Give the command a timeout of 600000 ms. It prints `✓`/`✗` per case, the steps that failed and
   the browser's console errors, and writes `<id>.mp4` (`.webm` without ffmpeg), `<id>.png` (the
   result card), `shots/<id>-NN.png` (step NN, annotated), `logs/` and merges the case into
   `<evidence dir>/evidence.json`. Exit code 1 means a case did not pass.
6. **Review every take.** Open `<id>.png` and the `shots/` of the case with the Read tool. Each step
   must show what its caption says, the outlined target must be the right element, the case must
   prove its expected result, and nothing personal may be readable. A step that failed because of
   the storyboard (wrong selector, page still loading — add a `wait`) → fix the storyboard and
   re-record. A step that failed because of the code → fix the code, re-run the tests, re-record.
   Browser console errors listed for a case are worth a look. Expect 1–3 takes.
7. **Check coverage**: every acceptance case has a case of its own; every journey of the spec has a
   flow.
8. **Record the presentation**, once every case is recorded (and again after any re-take):
   ```bash
   node <skill dir>/scripts/record-evidence.mjs <evidence dir>/storyboard.json --summary
   ```
   It writes `SUMMARY.mp4` and `SUMMARY.png` and the chapters into evidence.json, from what was
   recorded (it re-runs nothing). Watch a few frames of it with the Read tool on `SUMMARY.png`.
9. **Clean up**: stop every server you started. Evidence files stay in the evidence dir only —
   never add them to the repository.
10. **Report** a `## Test evidence` section: per case its id, kind, title, status and video file; the
   acceptance cases that are blocked or without evidence, and why; the full-suite results per
   repository (counts); any pre-existing failure, or console error the change causes.

## Storyboard

```json
{
  "title": "TASK-12 · Filter proforma invoices by status",
  "label": "TASK-12 · Implementation evidence",
  "base_url": "http://localhost:5173",
  "api_base": "http://localhost:8080",
  "repos": ["/abs/worktree/billing", "/abs/worktree/billing-frontend"],
  "cases": [
    {"id": "FLOW-1", "kind": "flow", "title": "Finance finds the paid proforma invoices of a customer",
     "covers": ["ABC-TC-001", "ABC-TC-003"], "given": "Signed in as Finance; customer PT Contoh has 2 paid and 1 unpaid invoice",
     "expected": "The list shows only the customer's paid invoices, and the detail page agrees",
     "steps": [
       {"action": "goto", "target": "/login", "caption": "Sign in as Finance"},
       {"action": "fill", "target": "label=Email", "value": "${TEST_USER}", "caption": "Test user"},
       {"action": "fill", "target": "label=Password", "value": "${TEST_PASSWORD}", "caption": "Password"},
       {"action": "click", "target": "role=button[name=\"Sign in\"]", "caption": "Sign in"},
       {"action": "click", "target": "text=Proforma Invoices", "caption": "Open Proforma Invoices from the menu",
        "screen": "The Proforma Invoices screen",
        "explain": [{"target": "label=Status", "label": "Status filter (new)"}, {"target": "table", "label": "Invoice list"}]},
       {"action": "select", "target": "label=Status", "value": "Paid", "caption": "Filter by Paid"},
       {"action": "expect_count", "target": "[data-testid=invoice-row]", "value": "2", "caption": "The two paid invoices remain"},
       {"action": "click", "target": "text=INV-001", "caption": "Open the first invoice"},
       {"action": "expect_text", "value": "Paid", "caption": "The detail page shows it as Paid"}
     ]},
    {"id": "FLOW-2", "kind": "flow", "title": "The 07:00 scheduler sends one alert and never retries", "covers": ["ABC-TC-007"],
     "given": "The reconcile job fails at the 07:00 run", "expected": "Exactly one alert is sent; no retry follows",
     "steps": [
       {"action": "start", "name": "alerts", "command": "node mock-alert-receiver.js", "cwd": "/abs/worktree/billing", "ready": "listening",
        "caption": "Start a mock alert receiver", "note": "Stands in for Slack: every alert lands here"},
       {"action": "start", "name": "scheduler", "command": "bundle exec rake scheduler:run", "cwd": "/abs/worktree/billing",
        "env": {"FAKE_NOW": "07:00", "ALERT_WEBHOOK_URL": "http://localhost:18282/alert"}, "ready": "scheduler started",
        "caption": "Run the scheduler with its clock at 07:00"},
       {"action": "watch", "process": "scheduler", "until": "alert sent", "caption": "The 07:00 run sends its alert"},
       {"action": "expect_log", "process": "alerts", "pattern": "ALERT received", "count": 1, "within_s": 90,
        "caption": "Exactly one alert, even 90 s later — no retry"},
       {"action": "expect_log", "process": "scheduler", "pattern": "retry", "count": 0, "caption": "The scheduler never retries"},
       {"action": "run", "command": "psql \"$TEST_DATABASE_URL\" -c \"select count(*) from alert_logs\"", "cwd": "/abs/worktree/billing",
        "caption": "One alert is logged in the database"}
     ]},
    {"id": "SUITE-billing-frontend", "kind": "suite", "title": "Full unit test suite of billing-frontend",
     "steps": [{"action": "run", "command": "npm test -- --reporter=dot", "cwd": "/abs/worktree/billing-frontend",
                "caption": "All unit tests", "suite": "billing-frontend", "timeout_s": 540}]},
    {"id": "ABC-TC-001", "kind": "acceptance", "title": "Finance filters the list by Paid", "covers": ["ABC-TC-001"],
     "given": "Three proforma invoices exist: two Paid, one Unpaid",
     "expected": "Only the two paid invoices are listed",
     "steps": [
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
| `run` | `command`, `cwd` (absolute), `env`, `highlight` (regex), `suite` (repository name, for a full-suite run), `timeout_s` (default 480), `live` | in a flow or acceptance case: **live** — a terminal window, the command typed, its output streaming, the real exit code. In a `suite` case it runs **before** the video (`live: true` to show it); its card has the lines matching `highlight`, test counts and duration. Full log in `logs/`; `suite` makes it the suite result of that repository in evidence.json, with real counts |
| `start` | `name`, `command`, `cwd`, `env`, `ready` (regex: the line that says it is up), `timeout_s` (default 120), `wait_s` (without `ready`, default 3) | starts a process of the system in the background — server, worker, scheduler, consumer, mock receiver — in a live terminal until it is ready; it runs until `stop` or the end of the case. Log in `logs/<case>-<name>.log` |
| `watch` | `process`, `until` (regex), `timeout_s` (default 60) | the process's live log until a new line matches — ✓ Verified, the line highlighted |
| `expect_log` | `process`, `pattern` (regex), `count` (exactly) / `min` / `max` (default: at least 1), `within_s` (watch this much longer first, default 5), `since` (`"check"`: only lines since the previous check; default: since the process started) | the live log, then the count of matching lines — e.g. "alert sent" exactly 1, "retry" 0 |
| `stop` | `process` | stops it, its last output |
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

A case takes `id`, `kind` (`flow`, `acceptance`, `suite`; inferred from a `FLOW-`/`SUITE-` id), `title`,
`covers`, `given`, `expected`, `notes`, `blocked` (why part of it cannot run — the case is then
Blocked) and `steps`. `criteria` (top level, `[{"id": "AC-1", "text": "…"}]`) lists the acceptance
criteria when the task has no ATDD cases.

Every step takes `caption`, `note`, `hold_ms` (how long it stays on screen) and `monitor` (a
process name: its live log shows in a drawer beside the app from this step on; `false` hides it). A browser step also
takes `explain` (`[{target, label}]`: after the step, the screen's regions are outlined, numbered
and listed with their labels), `screen` (the caption for that explanation, e.g. "The Proforma
Invoices screen") and `zoom` (`true` to zoom in on the target, `false` to never; by default
small fields zoom while they are filled, chosen, checked or uploaded). The storyboard's `"zoom":
false` turns zooming off.
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
- One case per recorder call. Unit-test `run` parts belong in the `SUITE` cases, not in flows or
  acceptance cases.
