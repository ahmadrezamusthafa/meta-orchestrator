---
name: test-evidence-video
description: "Test a finished change completely and record the proof as video: run every affected test suite for real, then record one narrated video per test case (title card, the test commands with their real output and exit code, browser walkthroughs of the running app with captions and highlighted targets, result card) plus evidence.json. Built into the orchestrator; the default evidence skill of the Implementation stage."
---

# test-evidence-video — test it completely, prove it on video

The evidence is only worth something if it is real: commands run by the recorder (it captures the
actual exit code and output), screens of the app actually running the change. Never hand-write a
result, never fake a passing run, never record against production.

## Workflow

1. **Find the test commands.** For every repository you changed, find how its tests really run:
   `Makefile`, `package.json` scripts, `go.mod`, `Gemfile`/`bin/rspec`, CI config
   (`.github/workflows`, `bitbucket-pipelines.yml`, `.gitlab-ci.yml`). Prefer the CI command.
2. **Run them completely first** (outside the recorder) — unit, integration and the specs or e2e
   tests that cover the change. Fix what your change broke and run again until green. A failure
   that also happens without your change (check the base branch or the CI history) is
   pre-existing: keep it, and say so in the case notes and the Summary.
3. **Plan the cases.** One case per acceptance test case (the ATDD cases when the brief points to
   them), plus one `SUITE` case for the full test run of each repository. Each case shows what
   proves it:
   - a `run` part with the narrowest command that exercises it (one spec file, `go test -run`,
     `vitest -t`) and a `highlight` regex for the lines that name it;
   - for a screen-driven case, browser parts on the app running locally from the task worktree —
     start it the way the repository documents (dev server, `docker compose`), against a local or
     test database with fake data. Skip the browser parts (keep the `run` part) when the app cannot
     start locally; say why in a `card` part.
4. **Write the storyboard** to `<evidence dir>/storyboard.json` (format below).
5. **Record one case per command**, so each call stays well inside the command time limit:
   ```bash
   node <skill dir>/scripts/record-evidence.mjs <evidence dir>/storyboard.json --case <id>
   ```
   Give the command a timeout of 600000 ms. It prints `✓`/`✗` per case and the parts that failed,
   writes `<id>.mp4` (or `.webm` without ffmpeg) and `<id>.png` (the result card), and merges the
   case into `<evidence dir>/evidence.json`. Exit code 1 means a case did not pass.
6. **Review.** Open every `<id>.png` with the Read tool: each part ✓, nothing personal readable.
   When a browser part failed because of the storyboard (wrong selector, page still loading),
   fix the storyboard and re-record that case. When it failed because of the code, fix the code,
   re-run the tests, and re-record. Expect 1–3 takes.
7. **Clean up**: stop any server you started. Evidence files stay in the evidence dir only — never
   add them to the repository.
8. **Report** a `## Test evidence` section: a table of case id, title, status and video file,
   the full-suite results per repository, and any pre-existing failure or skipped browser part.

## Storyboard

```json
{
  "title": "TASK-12 · Filter proforma invoices by status",
  "label": "TASK-12 · Implementation evidence",
  "base_url": "http://localhost:5173",
  "suites": [{"repository": "billing-frontend", "command": "npm test", "status": "passed", "passed": 412, "failed": 0}],
  "cases": [
    {"id": "SUITE-billing-frontend", "title": "Full unit test suite of billing-frontend",
     "steps": [{"action": "run", "command": "npm test -- --reporter=dot", "cwd": "/abs/worktree/billing-frontend",
                "caption": "All unit tests", "timeout_s": 540}]},
    {"id": "ABC-TC-001", "title": "Finance filters the list by Paid", "covers": ["ABC-TC-001"],
     "expected": "Only paid invoices are listed",
     "steps": [
       {"action": "run", "command": "npx vitest run src/views/proforma -t 'status filter'", "cwd": "/abs/worktree/billing-frontend",
        "caption": "Spec for the status filter", "highlight": "status filter"},
       {"action": "goto", "target": "/billing/proforma-invoices", "caption": "Open Proforma Invoices"},
       {"action": "select", "target": "label=Status", "value": "Paid", "caption": "Choose Paid in the Status filter"},
       {"action": "expect_text", "value": "PAID", "caption": "Only paid invoices are listed"}
     ]}
  ]
}
```

Parts (`steps`), played in order:

| action | fields | shows |
|---|---|---|
| `run` | `command`, `cwd` (absolute), `caption`, `highlight`, `timeout_s` (default 480) | runs it, terminal card with the output tail and real exit code; full log in `logs/` |
| `log` | `file` (relative to the evidence dir), `status`, `command`, `caption` | a saved log — only for a run that cannot be repeated by the recorder |
| `card` | `caption`, `body`, `kind` | a note, e.g. why a browser part was skipped |
| `goto` | `target` (path on `base_url`, or a full local URL) | the page |
| `click` `fill` `select` `check` | `target`, `value` | the action, target outlined, caption banner |
| `press` `wait` `expect_text` | `value` / `target` | waits, keys, the text that must appear |

Targets: `label=…`, `placeholder=…`, `testid=…`, `text=…`, `role=button[name="Save"]`, or CSS.
`${NAME}` in a target or value is read from the environment — use it for any login, never write a
credential into the storyboard. A case's `base_url` or `storage_state` overrides the storyboard's.

## Rules

- Fake data only: test databases, factories, seeds. Never a production or shared environment.
- The recorder blurs email-like and long-number text on screens and redacts secrets, emails and
  long numbers in logs; still check every preview, and change fixture data that looks real.
- Never print environment variables or `.env` files, and never put credentials in commands.
- One case per recorder call; keep `run` parts to the narrowest command that proves the case,
  except the `SUITE` cases.
