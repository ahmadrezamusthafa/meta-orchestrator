---
name: test-evidence-video
description: "Prove a finished feature works as specified, end to end: plan the evidence from the spec (user journeys, acceptance test cases, UI and API spec — not the diff), then record one narrated video per case in the running system — end-to-end user flows, one case per acceptance case (browser walkthroughs and API calls, every action and check annotated) and the full test suites as regression — plus a screenshot per step and evidence.json with the spec coverage. Built into the orchestrator; the default evidence skill of the Implementation stage."
---

# test-evidence-video — prove the feature works, end to end, as specified

Evidence is for a reviewer — a product owner, QA, the client — who did not write the change and
reads no code. It must show them that the feature **works completely, as specified**, in the
running system, the way its users and clients use it. So the evidence is planned from the
**spec**, not from the diff, and it is only worth something if it is real: screens and API calls
of the change actually running, commands actually run by the recorder. Never hand-write a
result, never fake a passing run, never record against production.

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
8. **Clean up**: stop every server you started. Evidence files stay in the evidence dir only —
   never add them to the repository.
9. **Report** a `## Test evidence` section: per case its id, kind, title, status and video file; the
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
       {"action": "click", "target": "text=Proforma Invoices", "caption": "Open Proforma Invoices from the menu"},
       {"action": "select", "target": "label=Status", "value": "Paid", "caption": "Filter by Paid"},
       {"action": "expect_count", "target": "[data-testid=invoice-row]", "value": "2", "caption": "The two paid invoices remain"},
       {"action": "click", "target": "text=INV-001", "caption": "Open the first invoice"},
       {"action": "expect_text", "value": "Paid", "caption": "The detail page shows it as Paid"}
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

A case takes `id`, `kind` (`flow`, `acceptance`, `suite`; inferred from a `FLOW-`/`SUITE-` id), `title`,
`covers`, `given`, `expected`, `notes`, `blocked` (why part of it cannot run — the case is then
Blocked) and `steps`. `criteria` (top level, `[{"id": "AC-1", "text": "…"}]`) lists the acceptance
criteria when the task has no ATDD cases.

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
- One case per recorder call. Unit-test `run` parts belong in the `SUITE` cases, not in flows or
  acceptance cases.
