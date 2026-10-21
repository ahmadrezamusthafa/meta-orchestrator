// Test evidence recorder: turns a storyboard of test cases into one narrated video per case, plus an
// annotated screenshot of every step. A case opens with a title card (Given / Expected / the steps
// to come / the commits under test), then plays its steps in order and ends with a result card:
//
//   - test commands, actually run here: real exit code, output, test counts, full log in logs/;
//   - API calls to the service running locally: request, response, and the checks on them;
//   - browser steps on the running app: the target outlined and the cursor on it *before* the
//     action, the screen *after* it, and a ✓ Verified banner for every check — each with a caption
//     and an optional note on what the screen proves.
//
// Results are merged into evidence.json next to the videos: per step its time in the video (so a
// viewer can jump to it), its screenshot and log; per case the commits under test and the browser's
// console errors; per suite the real test counts.
//
//   node record-evidence.mjs <storyboard.json> [--case <id> ...] [--summary]
//   node record-evidence.mjs <storyboard.json> --explore <path> [--explore <path> ...] [--session <name>]
//   node record-evidence.mjs <storyboard.json> --session <name> [--fresh]
//   node record-evidence.mjs <storyboard.json> --clear-sessions
//
// --summary (after the cases are recorded) makes SUMMARY.mp4: one video that presents the whole
// evidence — verdict, spec coverage, a chapter per case with its key steps, and the results.
// --explore opens pages of the running app (signed in with --session) and prints what is on them —
// headings, links, buttons, fields, tables — with the storyboard target for each, and a screenshot.
// --session alone signs in (or checks the saved session still works) and reports it.
//
// playwright-core is resolved from this script's folders, then from EVIDENCE_PLAYWRIGHT_DIR.
import { readFileSync, writeFileSync, mkdirSync, existsSync, renameSync, rmSync, readdirSync, createWriteStream } from 'node:fs'
import { join, dirname, basename, resolve as resolvePath, isAbsolute } from 'node:path'
import { createRequire } from 'node:module'
import { tmpdir } from 'node:os'
import { createHash } from 'node:crypto'
import { spawn, spawnSync } from 'node:child_process'

const args = process.argv.slice(2)
const VALUE_FLAGS = ['--case', '--explore', '--session']
const boardPath = args.find((a, i) => !a.startsWith('--') && !VALUE_FLAGS.includes(args[i - 1]))
if (!boardPath) {
  console.error('usage: node record-evidence.mjs <storyboard.json> [--case <id> ...] [--summary] | --explore <path> [--session <name>] | --session <name> [--fresh] | --clear-sessions')
  process.exit(2)
}
const flagValues = (name) => {
  const out = []
  for (let i = 0; i < args.length; i++) if (args[i] === name && args[i + 1]) out.push(args[++i])
  return out
}
const only = flagValues('--case')

const board = JSON.parse(readFileSync(boardPath, 'utf8'))
const boardDir = dirname(resolvePath(boardPath))
const outDir = resolvePath(board.out_dir || boardDir)
mkdirSync(outDir, { recursive: true })
const W = board.viewport?.width || 1280
const H = board.viewport?.height || 720
const STEP_TIMEOUT = 15000
const HOLD = { title: 3000, pre: 1100, step: 2400, check: 2600, run: 5500, log: 5000, api: 6000, card: 4000, annotate: 4000, result: 5000, ...(board.hold_ms || {}) }
// Steps drawn as a card (a full-screen layer) rather than on the app itself.
const CARD_ACTIONS = ['run', 'log', 'card', 'api', 'start', 'watch', 'expect_log', 'stop']
// Unit and integration test runners: their runs are regression (SUITE cases), never end-to-end evidence.
const UNIT_TEST = /\b(go test|jest|vitest|rspec|pytest|phpunit|mocha|karma|ava|minitest|rails test|mix test|cargo test|dotnet test|gradle(w)? test|mvn test|npm (run )?test|yarn test|pnpm test)\b/i

async function loadChromium() {
  for (const name of ['playwright-core', 'playwright']) {
    try { return (await import(name)).chromium } catch {}
  }
  const dir = process.env.EVIDENCE_PLAYWRIGHT_DIR
  if (dir) {
    for (const name of ['playwright-core', 'playwright']) {
      try { return createRequire(join(dir, 'package.json'))(name).chromium } catch {}
    }
  }
  throw new Error('playwright-core is not installed: set EVIDENCE_PLAYWRIGHT_DIR to a folder whose node_modules has playwright-core')
}

// ---------------------------------------------------------------------------------------------
// Redaction: the video and manifest never carry secrets or personal data, even when a test log does.
// ---------------------------------------------------------------------------------------------
const REDACT = [
  [/\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b/g, '«email»'],
  [/\b((?:api[_-]?key|token|secret|password|passwd|authorization|bearer|cookie)["']?\s*[:=]\s*["']?)[^\s"',;&]+/gi, '$1«redacted»'],
  [/\bBearer\s+[A-Za-z0-9._~+/=-]{8,}/g, 'Bearer «redacted»'],
  [/\b(?:sk|pk|ghp|gho|xox[abp])[-_][A-Za-z0-9_-]{10,}\b/g, '«redacted»'],
  [/\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{5,}/g, '«jwt»'],
  [/\b[a-z][a-z0-9+.-]*:\/\/[^\s:/@]+:[^\s@/]+@/gi, '«redacted-url»@'],
  [/\b\d{13,19}\b/g, '«number»'], // card, NIK and account numbers
  [/(?:\+62|\b08)\d{8,12}\b/g, '«phone»'],
]
const redact = (t) => REDACT.reduce((s, [re, to]) => s.replace(re, to), String(t ?? ''))
const esc = (t) => String(t ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
// eslint-disable-next-line no-control-regex
const stripAnsi = (t) => String(t ?? '').replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '')
const safeName = (id) => String(id).replace(/[^A-Za-z0-9._-]+/g, '_').slice(0, 80) || 'case'
// ${NAME}: a token captured by a session's sign-in (see Sessions), else the environment.
const vars = {}
const env = (v) => String(v ?? '').replace(/\$\{([A-Z0-9_]+)\}/g, (m, k) => vars[k] ?? process.env[k] ?? m)
const usesEnv = (v) => /\$\{[A-Z0-9_]+\}/.test(String(v ?? ''))
const pathOf = (u) => { try { const x = new URL(u); return x.pathname + (x.search ? '?…' : '') } catch { return String(u).split('?')[0] } }
const clock = () => new Date().toISOString().replace('T', ' ').slice(0, 19) + ' UTC'
const fmtSecs = (s) => (s >= 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s}s`)
// What a case proves: a whole user journey, one acceptance case, or a full suite run (regression).
const kindOf = (sc) => sc.kind || (/^suite/i.test(sc.id) ? 'suite' : /^flow/i.test(sc.id) ? 'flow' : 'acceptance')
const KIND_LABEL = { flow: 'End-to-end flow', acceptance: 'Acceptance case', suite: 'Regression suite' }
const stepCaption = (st) => redact(st.caption || st.command || `${st.action} ${st.target || st.value || ''}`.trim())

// ---------------------------------------------------------------------------------------------
// Cards: full-screen pages drawn into the recorded tab for the title, test runs, API calls, notes
// and the result.
// ---------------------------------------------------------------------------------------------
const STYLE = `
  * { box-sizing:border-box; margin:0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; }
  body { width:${W}px; height:${H}px; overflow:hidden; background:linear-gradient(135deg,#0f172a,#1e293b); color:#f8fafc;
         display:flex; flex-direction:column; }
  .prog { height:4px; background:#1e293b; } .prog i { display:block; height:100%; background:#a78bfa; }
  .top { display:flex; align-items:center; gap:14px; padding:16px 28px; border-bottom:1px solid #334155; }
  .tag { background:#7c3aed; color:#fff; font-weight:800; font-size:14px; padding:5px 11px; border-radius:7px; white-space:nowrap; }
  .crumb { flex:1; min-width:0; font-size:14px; color:#94a3b8; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .stepno { font-size:13px; font-weight:800; color:#c4b5fd; letter-spacing:.06em; text-transform:uppercase; white-space:nowrap; }
  .pill { font-size:14px; font-weight:800; padding:6px 14px; border-radius:999px; white-space:nowrap; }
  .main { flex:1; min-height:0; padding:24px 40px; display:flex; flex-direction:column; gap:14px; }
  .kind { font-size:13px; font-weight:800; text-transform:uppercase; letter-spacing:.1em; color:#a78bfa; }
  .big { font-size:30px; font-weight:700; line-height:1.3; }
  .mid { font-size:22px; font-weight:700; line-height:1.3; }
  .sub { font-size:17px; color:#cbd5e1; line-height:1.5; white-space:pre-wrap; }
  .note { font-size:15px; color:#cbd5e1; line-height:1.45; border-left:3px solid #a78bfa; padding-left:12px; }
  .gw { display:grid; grid-template-columns:110px 1fr; gap:6px 14px; font-size:16px; line-height:1.45; }
  .gw b { color:#94a3b8; font-size:12px; text-transform:uppercase; letter-spacing:.1em; padding-top:3px; }
  .chips { display:flex; flex-wrap:wrap; gap:8px; }
  .chip { font-size:14px; color:#e2e8f0; background:#334155; border-radius:999px; padding:6px 12px; }
  .chip.ok { background:#064e3b; color:#a7f3d0; } .chip.bad { background:#7f1d1d; color:#fecaca; }
  .term { flex:0 1 auto; min-height:0; background:#020617; border:1px solid #334155; border-radius:12px; padding:12px 16px; overflow:hidden;
          font:13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; color:#cbd5e1; display:flex; flex-direction:column; justify-content:flex-end; }
  .out { white-space:pre-wrap; word-break:break-word; }
  .match { background:#1c1917; border:1px solid #a16207; border-radius:10px; padding:8px 14px; font:13px/1.5 ui-monospace, Menlo, monospace;
           color:#fde68a; white-space:pre-wrap; word-break:break-word; max-height:180px; overflow:hidden; }
  .match .lbl { font:800 11px/1 system-ui; color:#fbbf24; letter-spacing:.1em; text-transform:uppercase; margin-bottom:6px; display:block; }
  .cmd { color:#f8fafc; font-weight:700; margin-bottom:6px; }
  .hl { color:#fde68a; } .ok { color:#6ee7b7; } .bad { color:#fca5a5; } .dim { color:#64748b; }
  .cols { flex:1; min-height:0; display:grid; grid-template-columns:1fr 1fr; gap:16px; }
  .pane { min-height:0; display:flex; flex-direction:column; gap:8px; }
  .pane .term { flex:1; justify-content:flex-start; }
  .list { display:flex; flex-direction:column; gap:7px; overflow:hidden; }
  .row { display:flex; gap:12px; align-items:baseline; font-size:16px; color:#e2e8f0; }
  .row b { min-width:22px; } .row .t { margin-left:auto; color:#64748b; font-size:13px; white-space:nowrap; }
  .agenda { display:flex; flex-direction:column; gap:4px; font-size:15px; color:#cbd5e1; overflow:hidden; }
  .agenda span { color:#64748b; display:inline-block; min-width:26px; }
  .win { flex:1; min-height:0; display:flex; flex-direction:column; border:1px solid #334155; border-radius:12px; overflow:hidden; background:#020617;
         box-shadow:0 12px 32px rgba(0,0,0,.35); }
  .wbar { display:flex; align-items:center; gap:7px; padding:8px 12px; background:#0f172a; border-bottom:1px solid #1e293b; }
  .wbar i { width:11px; height:11px; border-radius:50%; display:inline-block; }
  .wttl { flex:1; text-align:center; font:12px ui-monospace, Menlo, monospace; color:#94a3b8; }
  .wst { font-size:12px; font-weight:700; white-space:nowrap; }
  .wscr { flex:1; min-height:0; overflow:hidden; padding:10px 14px; font:13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; color:#cbd5e1;
          white-space:pre-wrap; word-break:break-word; }
  .prompt { color:#a78bfa; font-weight:700; }
  .caret { display:inline-block; width:8px; height:15px; background:#cbd5e1; vertical-align:text-bottom; }
  .hit { background:#065f46; color:#ecfdf5; border-radius:3px; padding:0 3px; }
  .foot { padding:9px 28px; font-size:12px; color:#64748b; border-top:1px solid #334155; display:flex; gap:18px; white-space:nowrap; overflow:hidden; }
`
const TONE = {
  passed: ['#047857', '#d1fae5', '✓ Passed'], failed: ['#b91c1c', '#fee2e2', '✗ Failed'], blocked: ['#b45309', '#fef3c7', '⚠ Blocked'],
  running: ['#1d4ed8', '#dbeafe', '● Recording'],
}
function cardPage(sc, status, kind, body, n, total, envLine) {
  const t = TONE[status] || TONE.running
  const pct = total ? Math.round((n / total) * 100) : 0
  return `<!doctype html><html><head><meta charset="utf-8"><style>${STYLE}</style></head><body>
    <div class="prog"><i style="width:${pct}%"></i></div>
    <div class="top"><span class="tag">${esc(sc.id)}</span><span class="crumb">${esc(redact(board.title || ''))} › ${esc(redact(sc.title))}</span>
      ${n ? `<span class="stepno">Step ${n} of ${total}</span>` : ''}
      <span class="pill" style="background:${t[1]};color:${t[0]}">${t[2]}</span></div>
    <div class="main"><div class="kind">${esc(kind)}</div>${body}</div>
    <div class="foot"><span>${esc(board.label || 'Test evidence')}</span><span>${clock()}</span>${envLine ? `<span>${esc(envLine)}</span>` : ''}</div>
  </body></html>`
}
const noteHTML = (note) => (note ? `<div class="note">${esc(redact(note))}</div>` : '')

function lineClass(l, hl) {
  if (hl && hl.test(l)) return 'hl'
  if (/\b(fail(ed|ure|ures)?|error|panic)\b/i.test(l) && !/\b0 (failed|failures?|errors?)\b/i.test(l)) return 'bad'
  if (/\b(pass(ed)?|ok|success)\b|✓/i.test(l)) return 'ok'
  return ''
}
function termBody(st, command, lines, chips) {
  let hl = null
  try { hl = st.highlight ? new RegExp(st.highlight, 'i') : null } catch {}
  const matched = hl ? lines.filter((l) => hl.test(l)).slice(0, 8) : []
  const tail = lines.slice(matched.length ? -24 : -32).map((l) => {
    const cls = lineClass(l, hl)
    return cls ? `<span class="${cls}">${esc(l)}</span>` : esc(l)
  }).join('\n')
  return `<div class="mid">${esc(redact(st.caption || 'Test command'))}</div>${noteHTML(st.note)}
    ${matched.length ? `<div class="match"><span class="lbl">Lines that prove it</span>${matched.map(esc).join('\n')}</div>` : ''}
    <div class="term"><div class="cmd">$ ${esc(redact(command))}</div><div class="out">${tail}</div></div>
    <div class="chips">${chips.map(([c, cls]) => `<span class="chip ${cls || ''}">${esc(c)}</span>`).join('')}</div>`
}

// Test counts from the common runners' summaries: go test, jest/vitest, rspec, pytest, phpunit, mocha.
function countTests(lines) {
  const text = lines.join('\n')
  const goPass = (text.match(/^\s*--- PASS:/gm) || []).length
  const goFail = (text.match(/^\s*--- FAIL:/gm) || []).length
  const goSkip = (text.match(/^\s*--- SKIP:/gm) || []).length
  if (goPass + goFail) return { passed: goPass, failed: goFail, skipped: goSkip }
  const okPkg = (text.match(/^ok\s+\S+/gm) || []).length
  const failPkg = (text.match(/^FAIL\s+\S+/gm) || []).length
  if (okPkg + failPkg) return { passed: okPkg, failed: failPkg, unit: 'packages' }
  for (let i = lines.length - 1; i >= 0; i--) {
    const l = lines[i]
    let m = l.match(/(\d+) examples?, (\d+) failures?(?:, (\d+) pending)?/) // rspec
    if (m) return { passed: +m[1] - +m[2] - (+m[3] || 0), failed: +m[2], skipped: +m[3] || 0 }
    m = l.match(/OK \((\d+) tests?/) // phpunit
    if (m) return { passed: +m[1], failed: 0 }
    m = l.match(/Tests: (\d+), Assertions: \d+(?:, Errors: (\d+))?(?:, Failures: (\d+))?/)
    if (m) return { passed: +m[1] - (+m[2] || 0) - (+m[3] || 0), failed: (+m[2] || 0) + (+m[3] || 0) }
    const p = l.match(/(\d+) (?:passed|passing)/)
    const f = l.match(/(\d+) (?:failed|failing)/)
    if ((p || f) && /^\s*(Tests?\b|=+|\d+ (passing|passed|failed)|Ran\b)/i.test(l)) {
      const s = l.match(/(\d+) (?:skipped|pending|todo)/)
      return { passed: p ? +p[1] : 0, failed: f ? +f[1] : 0, skipped: s ? +s[1] : 0 }
    }
  }
  return null
}
const countLine = (c) => c && `${c.passed} ${c.unit === 'packages' ? 'packages ok' : 'passed'} · ${c.failed} failed${c.skipped ? ` · ${c.skipped} skipped` : ''}`

// ---------------------------------------------------------------------------------------------
// Browser steps: the real app under an annotation overlay — progress, a caption banner with the
// phase (Action / Done / Verified / Failed), outlined targets with labels and a cursor. The overlay
// is plain DOM with pointer-events off, re-drawn for every phase because a navigation removes it.
// ---------------------------------------------------------------------------------------------
const PHASE = {
  action: ['#f59e0b', 'Action'], done: ['#38bdf8', 'Done'], verified: ['#10b981', '✓ Verified'],
  failed: ['#ef4444', '✗ Failed'], info: ['#a78bfa', 'Look here'],
}
async function overlay(page, o) {
  const [color, label] = PHASE[o.phase] || PHASE.done
  await page.evaluate(({ o, color, label }) => {
    document.getElementById('__ev_overlay')?.remove()
    const root = document.createElement('div')
    root.id = '__ev_overlay'
    root.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483647;font-family:system-ui,-apple-system,sans-serif'
    const style = document.createElement('style')
    style.textContent = '@keyframes __ev_ripple{0%{transform:translate(-50%,-50%) scale(.3);opacity:.9}100%{transform:translate(-50%,-50%) scale(2.4);opacity:0}}' +
      '@keyframes __ev_pulse{0%,100%{box-shadow:0 0 0 0 rgba(0,0,0,0)}50%{box-shadow:0 0 0 6px var(--c)}}'
    root.appendChild(style)
    const el = (css, text) => { const d = document.createElement('div'); d.style.cssText = css; if (text != null) d.textContent = text; root.appendChild(d); return d }

    el(`position:fixed;left:0;top:0;height:4px;width:${o.total ? (o.n / o.total) * 100 : 0}%;background:${color}`) // progress

    const boxes = o.boxes || []
    if (boxes.length) el('position:fixed;inset:0;background:rgba(15,23,42,.18)')
    const pad = boxes.length > 1 ? 3 : 6
    boxes.forEach((b, i) => {
      const ring = el(`position:fixed;left:${b.x - pad}px;top:${b.y - pad}px;width:${b.width + pad * 2}px;height:${b.height + pad * 2}px;` +
        `border:3px solid ${color};border-radius:10px;--c:${color}55;animation:__ev_pulse 1.2s ease-in-out infinite;background:rgba(255,255,255,.04)`)
      if (boxes.length > 1) {
        // Several regions: a numbered marker on each, the labels in a legend in the banner, so
        // labels of neighbouring regions never collide.
        const mark = document.createElement('div')
        mark.style.cssText = `position:absolute;left:-12px;top:-12px;width:24px;height:24px;border-radius:50%;background:${color};color:#0f172a;` +
          'font-size:13px;font-weight:800;display:flex;align-items:center;justify-content:center;box-shadow:0 2px 8px rgba(0,0,0,.35)'
        mark.textContent = String(i + 1)
        ring.appendChild(mark)
      } else if (b.label) {
        // The label goes beside the box when there is room, so it never covers what it points at.
        const tag = document.createElement('div')
        const side = b.x + b.width + 260 < window.innerWidth
        const where = side ? 'left:100%;margin-left:8px;top:50%;transform:translateY(-50%)'
          : b.y > 34 ? 'left:-3px;bottom:100%;margin-bottom:4px' : 'left:-3px;top:100%;margin-top:4px'
        tag.style.cssText = `position:absolute;${where};background:${color};color:#0f172a;box-shadow:0 2px 8px rgba(0,0,0,.3);` +
          'font-size:13px;font-weight:800;padding:3px 8px;border-radius:6px;white-space:nowrap;max-width:420px;overflow:hidden;text-overflow:ellipsis'
        tag.textContent = b.label
        ring.appendChild(tag)
      }
    })

    // The cursor glides from where it was, like a real pointer; a click ripples when it arrives.
    if (o.cursor) {
      const { x, y } = o.cursor
      const from = o.cursorFrom
      const glide = from && (Math.abs(from.x - x) + Math.abs(from.y - y)) > 4
      if (o.click) el(`position:fixed;left:${x}px;top:${y}px;width:44px;height:44px;border-radius:50%;background:${color};opacity:0;` +
        `animation:__ev_ripple .9s ease-out ${glide ? '.75s' : '0s'} infinite`)
      const cur = el(`position:fixed;left:${(glide ? from.x : x) - 3}px;top:${(glide ? from.y : y) - 2}px;width:26px;height:30px;` +
        'filter:drop-shadow(0 2px 3px rgba(0,0,0,.4));transition:left .7s cubic-bezier(.4,0,.2,1), top .7s cubic-bezier(.4,0,.2,1)')
      cur.innerHTML = '<svg width="26" height="30" viewBox="0 0 26 30"><path d="M3 2 L3 24 L9 18.5 L13 27 L17 25.2 L13 16.8 L21 16.8 Z" fill="#fff" stroke="#0f172a" stroke-width="2" stroke-linejoin="round"/></svg>'
      if (glide) requestAnimationFrame(() => requestAnimationFrame(() => { cur.style.left = `${x - 3}px`; cur.style.top = `${y - 2}px` }))
    }

    // The log drawer: the live output of a process the case monitors, beside the app.
    if (o.console) {
      const con = el('position:fixed;right:16px;width:46%;height:30%;background:rgba(2,6,23,.94);border:1px solid #334155;border-radius:10px;' +
        'box-shadow:0 10px 30px rgba(0,0,0,.4);display:flex;flex-direction:column;overflow:hidden')
      con.id = '__ev_con'
      const tb = document.createElement('div')
      tb.style.cssText = 'padding:5px 10px;font:600 11px ui-monospace,Menlo,monospace;color:#94a3b8;border-bottom:1px solid #1e293b;display:flex;gap:6px;align-items:center'
      tb.innerHTML = '<i style="width:8px;height:8px;border-radius:50%;background:#10b981;display:inline-block"></i>'
      tb.appendChild(document.createTextNode(o.console.title))
      con.appendChild(tb)
      const ls = document.createElement('div')
      ls.id = '__ev_con_lines'
      ls.style.cssText = 'flex:1;min-height:0;overflow:hidden;padding:6px 10px;font:12px/1.45 ui-monospace,Menlo,monospace;color:#cbd5e1;' +
        'white-space:pre-wrap;word-break:break-word;display:flex;flex-direction:column;justify-content:flex-end'
      ls.innerHTML = o.console.html
      con.appendChild(ls)
    }

    // The banner goes where it covers no outlined target.
    const low = boxes.some((b) => b.y + b.height > window.innerHeight * 0.6) && !boxes.some((b) => b.y < window.innerHeight * 0.35)
    const bar = el(`position:fixed;left:16px;right:16px;${low ? 'top:16px' : 'bottom:16px'};background:rgba(15,23,42,.95);color:#f8fafc;` +
      `border-left:6px solid ${color};border-radius:10px;padding:11px 16px;box-shadow:0 10px 30px rgba(0,0,0,.35)`)
    const head = document.createElement('div')
    head.style.cssText = 'font-size:17px;font-weight:600;line-height:1.35'
    const where = document.createElement('span')
    where.style.cssText = 'float:right;margin-left:16px;font-size:12px;font-weight:700;color:#94a3b8;letter-spacing:.04em;padding-top:3px'
    // Where the viewer is: the case, the step, and the screen (its heading, else the page title).
    const screen = (document.querySelector('h1')?.textContent || document.title || '').replace(/\s+/g, ' ').trim().slice(0, 48)
    where.textContent = `${o.caseId} · Step ${o.n} of ${o.total}${screen ? ` · ${screen}` : ''}`
    head.appendChild(where)
    if (o.newScreen) {
      const ns = document.createElement('span')
      ns.style.cssText = 'background:#a78bfa;color:#0f172a;font-size:11px;font-weight:800;letter-spacing:.06em;padding:2px 7px;border-radius:5px;margin-right:10px;text-transform:uppercase'
      ns.textContent = 'New screen'
      head.appendChild(ns)
    }
    const tag = document.createElement('span')
    tag.style.cssText = `color:${color};font-size:12px;font-weight:800;letter-spacing:.08em;text-transform:uppercase;margin-right:10px`
    tag.textContent = label
    head.appendChild(tag)
    head.appendChild(document.createTextNode(o.caption))
    bar.appendChild(head)
    if (o.note) {
      const n = document.createElement('div')
      n.style.cssText = 'font-size:14px;color:#cbd5e1;margin-top:4px;line-height:1.4'
      n.textContent = o.note
      bar.appendChild(n)
    }
    if (boxes.length > 1 && boxes.some((b) => b.label)) {
      const legend = document.createElement('div')
      legend.style.cssText = 'display:flex;flex-wrap:wrap;gap:6px 16px;margin-top:7px;font-size:14px;color:#e2e8f0'
      boxes.forEach((b, i) => {
        if (!b.label) return
        const item = document.createElement('span')
        const num = document.createElement('b')
        num.style.cssText = `display:inline-flex;width:20px;height:20px;border-radius:50%;background:${color};color:#0f172a;font-size:12px;` +
          'align-items:center;justify-content:center;margin-right:6px'
        num.textContent = String(i + 1)
        item.appendChild(num)
        item.appendChild(document.createTextNode(b.label))
        legend.appendChild(item)
      })
      bar.appendChild(legend)
    }
    document.documentElement.appendChild(root)
    const con = document.getElementById('__ev_con')
    // the drawer sits just above the banner (at the bottom, when the banner is at the top); measured
    // once the overlay is in the page
    if (con) con.style.bottom = low ? '16px' : `${bar.offsetHeight + 28}px`
  }, { o: { ...o, caption: redact(o.caption), note: o.note ? redact(o.note) : '' }, color, label }).catch(() => {})
  await maskPII(page)
}

// Blurs every email address, NIK, card or phone number on the screen, so a test fixture that
// resembles real personal data never ends up readable in the video. Only the matched text is
// covered (a blur box in the overlay), so the sentence around it — often the proof — stays readable.
async function maskPII(page) {
  await page.evaluate(() => {
    const root = document.getElementById('__ev_overlay')
    if (!root) return
    const re = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\b\d{13,19}\b|(?:\+62|\b08)\d{8,12}\b/g
    const walker = document.createTreeWalker(document.body || document.documentElement, NodeFilter.SHOW_TEXT)
    const range = document.createRange()
    for (let n = walker.nextNode(); n; n = walker.nextNode()) {
      if (root.contains(n)) continue
      for (const m of (n.nodeValue || '').matchAll(re)) {
        range.setStart(n, m.index)
        range.setEnd(n, m.index + m[0].length)
        for (const r of range.getClientRects()) {
          const box = document.createElement('div')
          box.style.cssText = `position:fixed;left:${r.left - 2}px;top:${r.top - 1}px;width:${r.width + 4}px;height:${r.height + 2}px;` +
            'backdrop-filter:blur(7px);-webkit-backdrop-filter:blur(7px);background:rgba(148,163,184,.35);border-radius:4px'
          root.appendChild(box)
        }
      }
    }
    for (const el of document.querySelectorAll('input, textarea')) {
      if (new RegExp(re.source).test(el.value || '')) el.style.filter = 'blur(6px)'
    }
  }).catch(() => {})
}

function candidates(scope, target) {
  const t = String(target || '').trim()
  const m = t.match(/^(label|placeholder|testid|text|role)=(.*)$/s)
  if (!m) return [scope.locator(t)]
  const [, kind, v] = m
  if (kind === 'label') return [scope.getByLabel(v, { exact: false })]
  if (kind === 'placeholder') return [scope.getByPlaceholder(v, { exact: false })]
  if (kind === 'testid') return [scope.locator(`[data-testid="${v}"], [data-test-id="${v}"], [data-test="${v}"], [data-qa="${v}"], [data-cy="${v}"], [id="${v}"]`)]
  if (kind === 'text') return [scope.getByText(v, { exact: false })]
  const rm = v.match(/^([a-z]+)(?:\[name=["'](.*)["']\])?$/)
  return rm ? [scope.getByRole(rm[1], rm[2] ? { name: rm[2], exact: false } : {})] : [scope.locator(v)]
}

async function findTarget(page, target, timeout = STEP_TIMEOUT) {
  const deadline = Date.now() + timeout
  for (;;) {
    for (const frame of page.frames()) {
      for (const loc of candidates(frame, target)) {
        const first = loc.first()
        if (await first.isVisible().catch(() => false)) return first
      }
    }
    if (Date.now() > deadline) throw new Error(`could not find ${target} on the screen within ${Math.round(timeout / 1000)}s`)
    await page.waitForTimeout(300)
  }
}

async function boxOf(loc) {
  if (!loc) return null
  await loc.scrollIntoViewIfNeeded({ timeout: 2000 }).catch(() => {})
  return loc.boundingBox().catch(() => null)
}
const center = (b) => (b ? { x: Math.round(b.x + b.width / 2), y: Math.round(b.y + b.height / 2) } : null)

async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
  await page.waitForFunction(() => document.body && document.body.innerText.trim().length > 0, null, { timeout: 5000 }).catch(() => {})
}

async function poll(page, ms, test, message) {
  const deadline = Date.now() + ms
  for (;;) {
    const v = await test().catch(() => null)
    if (v) return v
    if (Date.now() > deadline) throw new Error(typeof message === 'function' ? await message() : message)
    await page.waitForTimeout(300)
  }
}

// Zoom: like a screen recorder's auto-zoom, the page zooms in on a small field while it is filled
// or chosen, and back out after. Only the page's body is scaled; the annotation overlay is not.
const ZOOM = board.zoom === false ? 0 : Number(board.zoom_scale) || 1.6
function wantZoom(st, b) {
  if (!b || !ZOOM || st.zoom === false) return false
  if (st.zoom === true) return true
  return ['fill', 'select', 'check', 'uncheck', 'upload'].includes(st.action) && b.width < W * 0.35 && b.height < H * 0.2
}
async function zoomIn(page, b) {
  await page.evaluate(({ b, s }) => {
    const body = document.body
    body.style.transition = 'transform .55s cubic-bezier(.4,0,.2,1)'
    body.style.transformOrigin = `${b.x + b.width / 2 + window.scrollX}px ${b.y + b.height / 2 + window.scrollY}px`
    body.style.transform = `scale(${s})`
  }, { b, s: ZOOM }).catch(() => {})
  await page.waitForTimeout(600)
}
async function zoomOut(page) {
  const zoomed = await page.evaluate(() => {
    const body = document.body
    if (!body.style.transform) return false
    body.style.transform = ''
    return true
  }).catch(() => false)
  if (zoomed) await page.waitForTimeout(550)
}

// browserStep plays one browser step. An action on a target is shown twice: the target outlined
// with the cursor on it (what is about to happen), then the screen after it. Checks end Verified.
async function browserStep(page, ctx, st) {
  const url = (p) => (/^https?:\/\//.test(p) ? p : (ctx.base || '').replace(/\/$/, '') + (p.startsWith('/') ? p : '/' + p))
  const secret = st.secret || usesEnv(st.value)
  const act = async (loc, fn, click) => {
    let b = await boxOf(loc)
    const zoom = wantZoom(st, b)
    if (zoom) {
      await zoomIn(page, b)
      b = (await loc.boundingBox().catch(() => null)) || b
    }
    const from = ctx.cursor
    ctx.cursor = center(b) || ctx.cursor
    await ctx.show({ phase: 'action', boxes: b ? [b] : [], cursor: ctx.cursor, cursorFrom: from, click }, HOLD.pre + (from ? 500 : 0))
    await fn(loc)
    await settle(page)
    if (zoom) {
      await page.waitForTimeout(500) // the filled value, still zoomed in
      await zoomOut(page)
    }
    const ab = await boxOf(await findTarget(page, st.target, 1200).catch(() => null))
    return { phase: 'done', boxes: ab ? [ab] : [], cursor: center(ab) || ctx.cursor }
  }
  const verified = async (loc, label) => ({ phase: 'verified', boxes: [await boxOf(loc)].filter(Boolean).map((b) => ({ ...b, label })) })
  switch (st.action) {
    case 'goto': {
      await zoomOut(page)
      const res = await page.goto(url(env(st.target || st.value)), { waitUntil: 'domcontentloaded', timeout: 30000 })
      if (res && res.status() >= 400) throw new Error(`HTTP ${res.status()} at ${st.target}`)
      await settle(page)
      return { phase: 'done', boxes: [] }
    }
    case 'click': return act(await findTarget(page, st.target), (l) => l.click({ timeout: 5000 }), true)
    case 'dblclick': return act(await findTarget(page, st.target), (l) => l.dblclick({ timeout: 5000 }), true)
    case 'hover': return act(await findTarget(page, st.target), (l) => l.hover({ timeout: 5000 }))
    case 'check': return act(await findTarget(page, st.target), (l) => l.check({ timeout: 5000 }), true)
    case 'uncheck': return act(await findTarget(page, st.target), (l) => l.uncheck({ timeout: 5000 }), true)
    case 'fill': {
      const value = env(st.value)
      return act(await findTarget(page, st.target), async (l) => {
        await l.fill('', { timeout: 5000 })
        // Typed key by key so the video shows the input; a credential is filled at once and blurred.
        if (secret || value.length > 80) await l.fill(value, { timeout: 5000 })
        else await l.pressSequentially(value, { delay: 45, timeout: 15000 })
        if (secret) await l.evaluate((el) => { el.style.filter = 'blur(6px)' }).catch(() => {})
      })
    }
    case 'select': {
      const value = env(st.value)
      return act(await findTarget(page, st.target), (l) =>
        l.selectOption({ label: value }, { timeout: 5000 }).catch(() => l.selectOption(value, { timeout: 5000 })), true)
    }
    case 'upload': {
      const file = isAbsolute(st.value) ? st.value : join(boardDir, st.value)
      return act(await findTarget(page, st.target), (l) => l.setInputFiles(file, { timeout: 5000 }))
    }
    case 'press': {
      await ctx.show({ phase: 'action', boxes: [], cursor: ctx.cursor }, HOLD.pre)
      await page.keyboard.press(st.value || 'Enter')
      await settle(page)
      return { phase: 'done', boxes: [] }
    }
    case 'scroll': {
      if (st.target) {
        const l = await findTarget(page, st.target)
        return { phase: 'done', boxes: [await boxOf(l)].filter(Boolean) }
      }
      await page.mouse.wheel(0, parseInt(st.value || '600', 10) || 600)
      await page.waitForTimeout(500)
      return { phase: 'done', boxes: [] }
    }
    case 'wait':
      if (st.target) return { phase: 'done', boxes: [await boxOf(await findTarget(page, st.target, 20000))].filter(Boolean) }
      await page.waitForTimeout(Math.min(parseInt(st.value || '1000', 10) || 1000, 10000))
      return { phase: 'done', boxes: [] }
    case 'expect_text': {
      const want = env(st.value)
      const scope = st.target ? await findTarget(page, st.target, 20000) : page
      const l = await poll(page, 20000, async () => {
        const f = scope.getByText(want, { exact: false }).first()
        return (await f.isVisible()) ? f : null
      }, `expected text "${redact(want)}" did not appear${st.target ? ` in ${st.target}` : ''}`)
      return verified(l, st.label || `“${redact(want).slice(0, 60)}”`)
    }
    case 'expect_no_text': {
      const want = env(st.value)
      await poll(page, 10000, async () => !(await page.getByText(want, { exact: false }).first().isVisible()), `text "${redact(want)}" is still on the screen`)
      return { phase: 'verified', boxes: [] }
    }
    case 'expect_visible': return verified(await findTarget(page, st.target, 20000), st.label || '')
    case 'expect_hidden': {
      await poll(page, 10000, async () => !(await findTarget(page, st.target, 0).catch(() => null)), `${st.target} is still visible`)
      return { phase: 'verified', boxes: [] }
    }
    case 'expect_url': {
      const want = env(st.value)
      await poll(page, 10000, async () => page.url().includes(want), () => `the page is ${pathOf(page.url())}, not ${want}`)
      return { phase: 'verified', boxes: [] }
    }
    case 'expect_count': {
      const want = parseInt(st.value, 10)
      const loc = candidates(page, st.target)[0]
      await poll(page, 10000, async () => (await loc.count()) === want, async () => `expected ${want} × ${st.target}, found ${await loc.count().catch(() => '?')}`)
      const boxes = []
      for (let i = 0; i < Math.min(want, 12); i++) {
        const b = await loc.nth(i).boundingBox().catch(() => null)
        if (b) boxes.push({ ...b, label: '' })
      }
      return { phase: 'verified', boxes }
    }
    case 'annotate': {
      const boxes = []
      for (const a of st.targets || [{ target: st.target, label: st.label }]) {
        const b = await boxOf(await findTarget(page, a.target, 8000))
        if (b) boxes.push({ ...b, label: a.label || '' })
      }
      return { phase: 'info', boxes, hold: HOLD.annotate }
    }
    default: throw new Error(`unknown action ${st.action}`)
  }
}

// ---------------------------------------------------------------------------------------------
// Processes: test commands, and the system's own processes — a server, a worker, a scheduler, a
// consumer, a mock receiver for alerts — run for real, every line redacted and kept in logs/.
// In a flow or acceptance case they run live in the video, in a terminal window: the command is
// typed, its output streams as it happens, and a background process's log can be watched for
// what the change must (or must not) do. Suite runs go first, outside the video (see recordCase).
// ---------------------------------------------------------------------------------------------
const allProcs = new Set()
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

function spawnProc(st, logPath, background = false) {
  const started = Date.now()
  const startedAt = clock()
  const out = createWriteStream(logPath)
  out.write(`$ ${redact(st.command)}\n# cwd: ${st.cwd || boardDir}\n# started: ${startedAt}\n\n`)
  const extra = Object.fromEntries(Object.entries(st.env || {}).map(([k, v]) => [k, env(v)]))
  const child = spawn(st.command, { cwd: st.cwd || boardDir, shell: true, detached: true,
    env: { ...process.env, ...extra, CI: '1', FORCE_COLOR: '0', NO_COLOR: '1' } })
  // count: lines ever written; lines: the last 4000 of them. A mark is a count.
  const h = { child, lines: [], count: 0, mark: 0, started, startedAt, done: false, code: null, timedOut: false, seconds: 0 }
  let partial = ''
  const flush = (text) => {
    const got = text.split('\n').map((l) => redact(stripAnsi(l)))
    out.write(got.join('\n') + '\n')
    h.lines.push(...got)
    h.count += got.length
    if (h.lines.length > 4000) h.lines.splice(0, h.lines.length - 4000)
  }
  const take = (buf) => {
    const text = partial + buf.toString()
    const cut = text.lastIndexOf('\n')
    if (cut < 0) { partial = text; return }
    partial = text.slice(cut + 1)
    flush(text.slice(0, cut))
  }
  child.stdout.on('data', take)
  child.stderr.on('data', take)
  const kill = (sig) => { try { process.kill(-child.pid, sig) } catch { try { child.kill(sig) } catch {} } }
  const timer = background ? null : setTimeout(() => { h.timedOut = true; kill('SIGTERM') }, Math.min((st.timeout_s || 480) * 1000, 30 * 60 * 1000))
  h.exit = new Promise((done) => child.on('close', (code, signal) => {
    if (timer) clearTimeout(timer)
    if (partial) flush(partial)
    h.seconds = Math.round((Date.now() - started) / 1000)
    h.code = signal ? null : code
    h.done = true
    allProcs.delete(h)
    out.end(`\n# exit code: ${signal ? `stopped (${signal})` : code} after ${fmtSecs(h.seconds)}\n`)
    done(h)
  }))
  h.since = (mark) => h.lines.slice(Math.max(0, mark - (h.count - h.lines.length)))
  h.stop = async () => {
    if (h.done) return
    kill('SIGTERM')
    await Promise.race([h.exit, sleep(4000)])
    if (!h.done) kill('SIGKILL')
    await Promise.race([h.exit, sleep(1000)])
  }
  allProcs.add(h)
  return h
}

async function runCommand(st, logPath) {
  const h = await spawnProc(st, logPath).exit
  return { code: h.code, timedOut: h.timedOut, seconds: h.seconds, lines: h.lines, startedAt: h.startedAt }
}

// A terminal window drawn in the video, filled live by termUpdate.
function terminalBody(st, title) {
  return `<div class="mid">${esc(redact(st.caption || st.command || ''))}</div>${noteHTML(st.note)}
    <div class="win"><div class="wbar"><i style="background:#ef4444"></i><i style="background:#f59e0b"></i><i style="background:#10b981"></i>
      <span class="wttl">${esc(title)}</span><span id="__st" class="wst"></span></div>
      <div class="wscr"><div id="__lines"></div></div></div>
    <div class="chips" id="__chips"></div>`
}
function termHTML({ cmd, typed, lines = [], hl, hit, max = 30 }) {
  let hlRe = null, hitRe = null
  try { hlRe = hl ? new RegExp(hl, 'i') : null } catch {}
  try { hitRe = hit ? new RegExp(hit, 'i') : null } catch {}
  const head = cmd == null ? '' : `<span class="prompt">$</span> ${esc(typed == null ? cmd : cmd.slice(0, typed))}${typed != null && typed < cmd.length ? '<span class="caret"></span>' : ''}\n`
  const body = lines.slice(-max).map((l) => {
    if (hitRe && hitRe.test(l)) return `<span class="hit">${esc(l)}</span>`
    const c = lineClass(l, hlRe)
    return c ? `<span class="${c}">${esc(l)}</span>` : esc(l)
  }).join('\n')
  return head + body
}
async function termUpdate(frame, html, state, chips) {
  await frame.evaluate(({ html, state, chips }) => {
    const l = document.getElementById('__lines')
    if (l) l.innerHTML = html
    const st = document.getElementById('__st')
    if (st && state) { st.textContent = state[0]; st.style.color = state[1] }
    const c = document.getElementById('__chips')
    if (c && chips) c.innerHTML = chips
  }, { html, state, chips: chips ? chips.map(([t, cls]) => `<span class="chip ${cls || ''}">${esc(t)}</span>`).join('') : null }).catch(() => {})
}
async function typeCommand(frame, cmd) {
  const n = Math.min(cmd.length, 36)
  for (let k = 1; k <= n; k++) {
    await termUpdate(frame, termHTML({ cmd, typed: Math.ceil((k * cmd.length) / n) }))
    await sleep(28)
  }
  await sleep(250)
}
const RUNNING = (h) => [`● running ${fmtSecs(Math.round((Date.now() - h.started) / 1000))}`, '#60a5fa']

// API calls to the service running locally: the request (credentials come from the environment and
// are never shown), the response and the checks on it. The redacted exchange is saved in logs/.
// With the case's session, the call is made as the signed-in user: the session's api_headers and
// its cookies for the API's host are added, unless the step sets them itself (or "session": false).
async function apiCall(st, base, logPath, sess) {
  const method = String(st.method || 'GET').toUpperCase()
  const raw = env(st.url || st.target)
  const target = /^https?:\/\//.test(raw) ? raw : String(base || '').replace(/\/$/, '') + (raw.startsWith('/') ? raw : '/' + raw)
  const headers = Object.fromEntries(Object.entries(st.headers || {}).map(([k, v]) => [k, env(v)]))
  if (sess && st.session !== false) {
    const has = (k) => Object.keys(headers).some((h) => h.toLowerCase() === k.toLowerCase())
    for (const [k, v] of Object.entries(sess.apiHeaders)) if (!has(k)) headers[k] = env(v)
    const cookie = cookieHeader(sess.storage, target)
    if (cookie && !has('cookie')) headers.Cookie = cookie
  }
  let body
  if (st.body !== undefined) {
    body = typeof st.body === 'string' ? env(st.body) : env(JSON.stringify(st.body))
    if (typeof st.body !== 'string' && !Object.keys(headers).some((k) => k.toLowerCase() === 'content-type')) headers['Content-Type'] = 'application/json'
  }
  const started = Date.now()
  const res = await fetch(target, { method, headers, body, signal: AbortSignal.timeout((st.timeout_s || 30) * 1000) })
  const text = await res.text()
  const ms = Date.now() - started
  let pretty = text
  try { pretty = JSON.stringify(JSON.parse(text), null, 2) } catch {}
  pretty = redact(pretty)
  // The request body as the storyboard wrote it: ${NAME} placeholders stay unexpanded on screen.
  const reqShown = st.body === undefined ? '' : redact(typeof st.body === 'string' ? st.body : JSON.stringify(st.body, null, 2))

  const checks = []
  const want = st.expect_status ?? null
  const statusOK = want == null ? res.status < 400 : [].concat(want).map(Number).includes(res.status)
  checks.push([want == null ? `status ${res.status} is not an error` : `status ${res.status} (expected ${[].concat(want).join(' or ')})`, statusOK])
  for (const s of [].concat(st.expect_body ?? [])) checks.push([`response has “${redact(env(s))}”`, text.includes(env(s))])
  for (const s of [].concat(st.expect_no_body ?? [])) checks.push([`response has no “${redact(env(s))}”`, !text.includes(env(s))])

  const u = new URL(target)
  const shownHeaders = Object.keys(headers).map((k) => `${k}: ${/auth|token|cookie|key|secret|session/i.test(k) ? '«redacted»' : redact(headers[k])}`)
  writeFileSync(logPath, `${method} ${redact(u.pathname + u.search)}\n${shownHeaders.join('\n')}${reqShown ? `\n\n${reqShown}` : ''}\n\n` +
    `# HTTP ${res.status} ${res.statusText} in ${ms} ms\n\n${pretty}\n\n# checks\n${checks.map(([c, ok]) => `${ok ? '✓' : '✗'} ${c}`).join('\n')}\n`)
  return { method, path: pathOf(target), shownPath: redact(u.pathname + u.search), status: res.status, statusText: res.statusText, ms, pretty, reqShown, headerNames: Object.keys(headers), checks }
}

function apiBody(st, r) {
  const cut = (t, n) => { const l = t.split('\n'); return l.length > n ? l.slice(0, n).join('\n') + `\n… ${l.length - n} more lines (full response in the log)` : t }
  let resp = esc(cut(r.pretty, 26))
  for (const s of [].concat(st.expect_body ?? []).map((x) => redact(env(x))).filter(Boolean)) resp = resp.split(esc(s)).join(`<span class="hl">${esc(s)}</span>`)
  const ok = r.status < 400
  return `<div class="mid">${esc(redact(st.caption || `${r.method} ${r.shownPath}`))}</div>${noteHTML(st.note)}
    <div class="cols">
      <div class="pane"><div class="kind" style="color:#94a3b8">Request</div>
        <div class="term"><div class="cmd">${esc(r.method)} ${esc(r.shownPath)}</div><div class="out">${r.headerNames.length ? `<span class="dim">headers: ${esc(r.headerNames.join(', '))}</span>\n` : ''}${esc(cut(r.reqShown, 22))}</div></div></div>
      <div class="pane"><div class="kind" style="color:#94a3b8">Response</div>
        <div class="term"><div class="cmd"><span class="${ok ? 'ok' : 'bad'}">HTTP ${r.status} ${esc(r.statusText)}</span> <span class="dim">· ${r.ms} ms</span></div><div class="out">${resp}</div></div></div>
    </div>
    <div class="chips">${r.checks.map(([c, ok]) => `<span class="chip ${ok ? 'ok' : 'bad'}">${ok ? '✓' : '✗'} ${esc(c)}</span>`).join('')}</div>`
}

// ---------------------------------------------------------------------------------------------
// Cards during a case: on the app, a card is a full-screen layer over it (an iframe), so the app
// keeps its state — the filled form, the open dialog — for the browser steps after the card.
// ---------------------------------------------------------------------------------------------
const onApp = (page) => !/^(about:blank|data:)/.test(page.url())
async function showCard(page, html) {
  if (!onApp(page)) {
    await page.setContent(html)
    return page.mainFrame()
  }
  await page.evaluate((html) => {
    document.getElementById('__ev_card')?.remove()
    const f = document.createElement('iframe')
    f.id = '__ev_card'
    f.name = '__ev_card'
    f.style.cssText = 'position:fixed;inset:0;width:100vw;height:100vh;border:0;z-index:2147483647;background:#0f172a'
    f.srcdoc = html
    document.documentElement.appendChild(f)
  }, html)
  for (let i = 0; i < 40; i++) {
    const f = page.frame({ name: '__ev_card' })
    if (f && (await f.evaluate(() => document.readyState === 'complete' && !!document.body).catch(() => false))) return f
    await sleep(50)
  }
  return page.mainFrame()
}
async function hideCard(page) {
  await page.evaluate(() => document.getElementById('__ev_card')?.remove()).catch(() => {})
}

// ---------------------------------------------------------------------------------------------
// What is under test: the branch and commit of each repository a step runs in.
// ---------------------------------------------------------------------------------------------
function repoInfo(dirs) {
  const seen = new Map()
  for (const d of dirs) {
    if (!d || !existsSync(d)) continue
    const git = (...a) => spawnSync('git', ['-C', d, ...a], { encoding: 'utf8' })
    const top = git('rev-parse', '--show-toplevel').stdout?.trim()
    if (!top || seen.has(top)) continue
    seen.set(top, {
      name: basename(top),
      branch: git('rev-parse', '--abbrev-ref', 'HEAD').stdout?.trim() || '',
      commit: git('rev-parse', '--short', 'HEAD').stdout?.trim() || '',
      uncommitted: (git('status', '--porcelain', '--untracked-files=no').stdout || '').trim() !== '',
    })
  }
  return [...seen.values()]
}

function toMP4(webm, mp4) {
  if (spawnSync('ffmpeg', ['-version'], { stdio: 'ignore' }).status !== 0) return false
  const r = spawnSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', webm, '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', mp4])
  return r.status === 0
}

// ---------------------------------------------------------------------------------------------
// Sessions: sign in once, off camera, and reuse it. A storyboard's "sessions" name the test users
// (or roles) the cases act as; a case with "session": "<name>" starts already signed in, on the
// screen under test, instead of every video opening with the same login form. The session —
// cookies, local and session storage, a captured token — is saved outside the evidence directory
// (it is never published with the evidence), checked before each use and signed in again when it
// expired. Its cookies and token also sign the case's `api` calls.
// ---------------------------------------------------------------------------------------------
const SESSION_DIR = resolvePath(process.env.EVIDENCE_SESSION_DIR || board.session_dir ||
  join(tmpdir(), 'evidence-sessions', createHash('sha1').update(outDir).digest('hex').slice(0, 12)))
const SESSION_MAX_AGE_MIN = 120
// A URL that is a sign-in page: being sent there means the session is not (or no longer) valid.
const SIGNED_OUT = /(log-?in|sign-?in|sign_in|\/auth\b|\/sso\b|oauth|openid|\/session\/new)/i
const sessionFile = (name) => join(SESSION_DIR, safeName(name) + '.json')
const tokenVar = (name, cfg) => cfg.token_var || `${String(name).toUpperCase().replace(/[^A-Z0-9]+/g, '_')}_TOKEN`
const dig = (o, path) => String(path || '').split('.').filter(Boolean).reduce((x, k) => (x == null ? x : x[k]), o)
const contextOptions = (extra = {}) => ({
  viewport: { width: W, height: H }, ignoreHTTPSErrors: true, locale: board.locale || undefined, timezoneId: board.timezone || undefined, ...extra,
})
const expandAll = (o) => Object.fromEntries(Object.entries(o || {}).map(([k, v]) => [k, env(v)]))

// The Cookie header the browser would send to url, from a saved storage state.
function cookieHeader(storage, url) {
  let u
  try { u = new URL(url) } catch { return '' }
  const now = Date.now() / 1000
  return (storage?.cookies || []).filter((c) => {
    const d = String(c.domain || '').replace(/^\./, '')
    return (u.hostname === d || u.hostname.endsWith('.' + d)) && u.pathname.startsWith(c.path || '/') &&
      (!c.expires || c.expires < 0 || c.expires > now) && (!c.secure || u.protocol === 'https:' || u.hostname === 'localhost')
  }).map((c) => `${c.name}=${c.value}`).join('; ')
}

// sessionStorage is not part of Playwright's storage state: it is put back on each page load
// of its origin, without overwriting what the app set itself.
async function restoreSessionStorage(context, saved) {
  if (!saved.session_storage || !Object.keys(saved.session_storage).length) return
  await context.addInitScript((byOrigin) => {
    const items = byOrigin[location.origin]
    if (!items) return
    try { for (const [k, v] of Object.entries(items)) if (sessionStorage.getItem(k) == null) sessionStorage.setItem(k, v) } catch {}
  }, saved.session_storage)
}

async function sessionContext(browser, cfg, saved, extra = {}) {
  const headers = expandAll(cfg.headers)
  const context = await browser.newContext(contextOptions({ storageState: saved.storage, ...(Object.keys(headers).length ? { extraHTTPHeaders: headers } : {}), ...extra }))
  await restoreSessionStorage(context, saved)
  return context
}

// Signed in: the session's home page opens without a detour to a sign-in page (or, with
// "signed_in", shows that element — e.g. the user menu).
async function checkSignedIn(page, cfg, base) {
  const home = cfg.home || '/'
  const res = await page.goto(/^https?:\/\//.test(home) ? home : base.replace(/\/$/, '') + (home.startsWith('/') ? home : '/' + home),
    { waitUntil: 'domcontentloaded', timeout: 30000 }).catch((e) => { throw new Error(`could not open ${home}: ${String(e.message).split('\n')[0]}`) })
  await settle(page)
  if (res && (res.status() === 401 || res.status() === 403)) return `HTTP ${res.status()} at ${home}`
  if (cfg.signed_in) {
    const ok = await findTarget(page, cfg.signed_in, 10000).then(() => true, () => false)
    return ok ? '' : `${cfg.signed_in} is not on ${pathOf(page.url())}`
  }
  const where = pathOf(page.url())
  let re = SIGNED_OUT
  try { if (cfg.signed_out_url) re = new RegExp(cfg.signed_out_url, 'i') } catch {}
  return re.test(where) ? `sent to the sign-in page (${where})` : ''
}

// An API session signs in with api_login alone and has no app to open: no browser is used, and it
// is checked by calling "home" on the API with its token (a client-credentials token, for example).
const isApiSession = (cfg) => cfg.browser === false || (!!cfg.api_login && !cfg.login?.length && !cfg.cookies?.length &&
  !cfg.local_storage && !cfg.session_storage && !cfg.token_from && !(cfg.base_url || board.base_url))
// A captured token is reused only until shortly before the expires_in its login response gave.
const EXPIRY_MARGIN_MS = 30000
const expired = (saved) => !!saved.expires_at && Date.now() > Date.parse(saved.expires_at) - EXPIRY_MARGIN_MS

async function checkApiSession(name, cfg, token) {
  if (!cfg.home) return ''
  const apiBase = cfg.api_base || board.api_base || ''
  const target = /^https?:\/\//.test(cfg.home) ? cfg.home : String(apiBase).replace(/\/$/, '') + (cfg.home.startsWith('/') ? cfg.home : '/' + cfg.home)
  const tv = tokenVar(name, cfg)
  const tok = token ?? vars[tv]
  const headers = cfg.api_headers
    ? Object.fromEntries(Object.entries(cfg.api_headers).map(([k, v]) => [k, env(String(v).replaceAll(`\${${tv}}`, tok ?? ''))]))
    : tok ? { Authorization: `Bearer ${tok}` } : {}
  const res = await fetch(target, { headers, redirect: 'manual', signal: AbortSignal.timeout(30000) })
    .catch((e) => { throw new Error(`could not call ${pathOf(target)}: ${String(e.message).split('\n')[0]}`) })
  return res.status === 401 || res.status === 403 ? `HTTP ${res.status} at ${pathOf(target)}` : ''
}

// apiLogin calls the login API and returns when its token expires (from expires_in), if it said.
async function apiLogin(name, cfg, context, base) {
  const a = cfg.api_login
  const apiBase = a.base_url || cfg.api_base || board.api_base || base
  const raw = env(a.url)
  const target = /^https?:\/\//.test(raw) ? raw : String(apiBase).replace(/\/$/, '') + (raw.startsWith('/') ? raw : '/' + raw)
  const headers = expandAll(a.headers)
  let body
  if (a.body !== undefined) {
    body = typeof a.body === 'string' ? env(a.body) : env(JSON.stringify(a.body))
    if (typeof a.body !== 'string' && !Object.keys(headers).some((k) => k.toLowerCase() === 'content-type')) headers['Content-Type'] = 'application/json'
  }
  const res = await fetch(target, { method: String(a.method || 'POST').toUpperCase(), headers, body, redirect: 'manual', signal: AbortSignal.timeout(30000) })
  const text = await res.text()
  if (res.status >= 400) throw new Error(`api_login ${pathOf(target)}: HTTP ${res.status} ${redact(text).slice(0, 160)}`)
  let json = null
  try { json = JSON.parse(text) } catch {}
  if (a.token_path) {
    const tok = dig(json, a.token_path)
    if (tok == null || tok === '') throw new Error(`api_login ${pathOf(target)}: no token at ${a.token_path} in the response`)
    vars[tokenVar(name, cfg)] = String(tok)
  }
  const ttl = Number(json?.expires_in)
  const expiresAt = ttl > 0 ? new Date(Date.now() + ttl * 1000).toISOString() : undefined
  if (!context) return expiresAt
  const host = new URL(target)
  const cookies = (res.headers.getSetCookie?.() || []).map((c) => {
    const [nv, ...attrs] = c.split(';')
    const at = nv.indexOf('=')
    const path = attrs.map((x) => x.trim()).find((x) => /^path=/i.test(x))
    return at > 0 ? { name: nv.slice(0, at).trim(), value: nv.slice(at + 1).trim(), domain: host.hostname, path: path ? path.slice(5) : '/' } : null
  }).filter(Boolean)
  if (cookies.length) await context.addCookies(cookies)
  return expiresAt
}

// signIn establishes the session in a browser that is not recorded, by any mix of: an API login
// (api_login), cookies, local or session storage set directly, and the login steps played in the
// browser like a case's steps. It fails with a screenshot of where it stopped.
async function signIn(browser, name, cfg) {
  const base = cfg.base_url || board.base_url || ''
  const tv = tokenVar(name, cfg)
  if (isApiSession(cfg)) {
    let at = 'api_login'
    try {
      const expires_at = await apiLogin(name, cfg, null, base)
      at = 'the check that the token is accepted'
      const why = await checkApiSession(name, cfg)
      if (why) throw new Error(why)
      return { name, saved_at: new Date().toISOString(), api: true, expires_at, session_storage: {}, vars: vars[tv] ? { [tv]: vars[tv] } : {} }
    } catch (err) {
      throw new Error(`signing in as "${name}" failed at ${at}: ${redact(String(err?.message || err).split('\n')[0])}`)
    }
  }
  const headers = expandAll(cfg.headers)
  const context = await browser.newContext(contextOptions(Object.keys(headers).length ? { extraHTTPHeaders: headers } : {}))
  const page = await context.newPage()
  page.setDefaultTimeout(STEP_TIMEOUT)
  const failShot = join(SESSION_DIR, `${safeName(name)}-sign-in-failed.png`)
  let at = 'start'
  let expires_at
  try {
    if (cfg.api_login) { at = 'api_login'; expires_at = await apiLogin(name, cfg, context, base) }
    if (cfg.cookies?.length) {
      at = 'cookies'
      await context.addCookies(cfg.cookies.map((c) => ({ name: c.name, value: env(c.value),
        ...(c.domain ? { domain: c.domain, path: c.path || '/' } : { url: c.url || base }) })))
    }
    if (cfg.local_storage || cfg.session_storage) {
      at = 'storage'
      await page.goto(base, { waitUntil: 'domcontentloaded', timeout: 30000 })
      await page.evaluate(({ l, s }) => {
        for (const [k, v] of Object.entries(l)) localStorage.setItem(k, v)
        for (const [k, v] of Object.entries(s)) sessionStorage.setItem(k, v)
      }, { l: expandAll(cfg.local_storage), s: expandAll(cfg.session_storage) })
    }
    const ctx = { base, cursor: null, show: async () => {} }
    for (const [i, st] of (cfg.login || []).entries()) {
      at = `login step ${i + 1} (${stepCaption(st)})`
      await browserStep(page, ctx, { ...st, zoom: false })
    }
    await settle(page)
    if (cfg.token_from) {
      at = 'token_from'
      const [where, ...rest] = String(cfg.token_from).split(':')
      const key = rest.join(':')
      let v = where === 'cookie'
        ? (await context.cookies()).find((c) => c.name === key)?.value
        : await page.evaluate(({ where, key }) => (where === 'session_storage' ? sessionStorage : localStorage).getItem(key), { where, key })
      if (v != null && cfg.token_path) { try { v = dig(JSON.parse(v), cfg.token_path) } catch { v = null } }
      if (v == null || v === '') throw new Error(`no token in ${cfg.token_from}${cfg.token_path ? ` at ${cfg.token_path}` : ''}`)
      vars[tokenVar(name, cfg)] = String(v)
    }
    at = 'the check that the session is signed in'
    const why = await checkSignedIn(page, cfg, base)
    if (why) throw new Error(why)
    const session_storage = {}
    const origin = new URL(page.url()).origin
    const ss = await page.evaluate(() => Object.fromEntries(Object.entries(sessionStorage))).catch(() => ({}))
    if (Object.keys(ss).length) session_storage[origin] = ss
    const saved = { name, saved_at: new Date().toISOString(), expires_at, storage: await context.storageState(), session_storage, vars: vars[tv] ? { [tv]: vars[tv] } : {} }
    rmSync(failShot, { force: true })
    return saved
  } catch (err) {
    await maskedShot(page, failShot)
    console.log(`  session "${name}": sign-in stopped at ${at} — the screen it stopped on: ${failShot}`)
    throw new Error(`signing in as "${name}" failed at ${at}: ${redact(String(err?.message || err).split('\n')[0])}`)
  } finally {
    await context.close()
  }
}

async function maskedShot(page, path) {
  await page.evaluate(() => {
    if (document.getElementById('__ev_overlay')) return
    const root = document.createElement('div')
    root.id = '__ev_overlay'
    root.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483647'
    document.documentElement.appendChild(root)
  }).catch(() => {})
  await maskPII(page)
  await page.screenshot({ path }).catch(() => {})
}

// ensureSession returns the named session, signed in: the saved one when it is recent and still
// works, else a new sign-in (saved for the next recorder call).
const sessionCache = new Map()
async function ensureSession(browser, name, { fresh = false } = {}) {
  const cfg = (board.sessions || {})[name]
  if (!cfg) throw new Error(`no session named "${name}" in the storyboard's "sessions" (${Object.keys(board.sessions || {}).join(', ') || 'none defined'})`)
  if (!fresh && sessionCache.has(name)) return sessionCache.get(name)
  mkdirSync(SESSION_DIR, { recursive: true, mode: 0o700 })
  const file = sessionFile(name)
  const base = cfg.base_url || board.base_url || ''
  let saved = null
  let reused = false
  if (!fresh && existsSync(file)) {
    try { saved = JSON.parse(readFileSync(file, 'utf8')) } catch {}
    const age = saved ? (Date.now() - Date.parse(saved.saved_at)) / 60000 : Infinity
    if (saved && age < (cfg.max_age_min || SESSION_MAX_AGE_MIN)) {
      let why
      if (expired(saved)) {
        why = 'its token expired'
      } else if (isApiSession(cfg)) {
        why = await checkApiSession(name, cfg, saved.vars?.[tokenVar(name, cfg)]).catch((e) => String(e.message || e))
      } else {
        const context = await sessionContext(browser, cfg, saved)
        why = await checkSignedIn(await context.newPage(), cfg, base).catch((e) => String(e.message || e))
        await context.close()
      }
      reused = !why
      if (!reused) console.log(`  session "${name}": the saved one no longer works (${why}) — signing in again`)
    }
  }
  if (!reused) {
    console.log(`  session "${name}": signing in …`)
    saved = await signIn(browser, name, cfg)
    writeFileSync(file, JSON.stringify(saved), { mode: 0o600 })
  }
  Object.assign(vars, saved.vars || {})
  const tv = tokenVar(name, cfg)
  const apiHeaders = cfg.api_headers || (vars[tv] ? { Authorization: `Bearer \${${tv}}` } : {})
  const sess = { name, cfg, saved, storage: saved.storage, apiHeaders, reused }
  sessionCache.set(name, sess)
  return sess
}

// ---------------------------------------------------------------------------------------------
// Explore (--explore): open pages of the running app, signed in, and list what is on them with the
// storyboard target for each — so the storyboard is written from the real screen, not guessed.
// ---------------------------------------------------------------------------------------------
async function explorePages(browser, paths, sessName) {
  const sess = sessName ? await ensureSession(browser, sessName) : null
  const base = sess?.cfg.base_url || board.base_url || ''
  const context = sess ? await sessionContext(browser, sess.cfg, sess.saved) : await browser.newContext(contextOptions())
  const page = await context.newPage()
  const dir = join(SESSION_DIR, 'explore')
  mkdirSync(dir, { recursive: true, mode: 0o700 })
  try {
    for (const p of paths) {
      const url = /^https?:\/\//.test(p) ? p : base.replace(/\/$/, '') + (p.startsWith('/') ? p : '/' + p)
      const res = await page.goto(env(url), { waitUntil: 'domcontentloaded', timeout: 30000 }).catch((e) => { console.log(`✗ ${p}: ${String(e.message).split('\n')[0]}`); return undefined })
      if (res === undefined) continue
      await settle(page)
      await page.waitForTimeout(600)
      const map = await page.evaluate(pageMap)
      const shot = join(dir, `${safeName(p.replace(/^https?:\/\/[^/]+/, '') || 'home')}.png`)
      await maskedShot(page, shot)
      const signedOut = SIGNED_OUT.test(pathOf(page.url()))
      const out = [`\n━━ ${p} → ${pathOf(page.url())}${res ? ` (HTTP ${res.status()})` : ''} · "${map.title}"${sess ? ` · session ${sess.name}` : ''}`]
      if (signedOut && sess) out.push('  ⚠ this looks like a sign-in page: the session may not reach this screen (check the session\'s home / signed_in)')
      out.push(`  screenshot: ${shot}`)
      const sect = (title, rows) => { if (rows.length) out.push(`  ${title}:`, ...rows.map((r) => `    ${r}`)) }
      sect('headings', map.headings)
      sect('links', map.links.map((l) => `${l.target}  → ${l.href}`))
      sect('buttons', map.buttons)
      sect('fields', map.fields.map((f) => `${f.target}  (${f.kind}${f.options ? `: ${f.options.join(' | ')}` : ''})`))
      sect('tables', map.tables.map((t) => `${t.target}  ${t.rows} rows · columns: ${t.cols.join(' | ')}`))
      sect('test ids', map.testids)
      sect('dialogs', map.dialogs)
      console.log(redact(out.join('\n')))
    }
  } finally {
    await context.close()
  }
}

// Runs in the page: the visible landmarks of the screen, each with a target the recorder accepts.
function pageMap() {
  const vis = (el) => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 0 && r.height > 0 && s.visibility !== 'hidden' && s.display !== 'none' }
  const clean = (t, n = 60) => String(t || '').replace(/\s+/g, ' ').trim().slice(0, n)
  const q = (t) => JSON.stringify(t)
  const tid = (el) => ['data-testid', 'data-test-id', 'data-test', 'data-qa', 'data-cy'].map((a) => el.getAttribute(a)).find(Boolean)
  // A <label> wrapping its control: its own text, without the control's (a select's options).
  const labelText = (l) => {
    if (!l) return ''
    const c = l.cloneNode(true)
    c.querySelectorAll('input, select, textarea, button').forEach((x) => x.remove())
    return c.textContent
  }
  const labelOf = (el) => {
    const by = el.getAttribute('aria-labelledby')
    return clean(el.getAttribute('aria-label') || labelText(el.labels?.[0]) || (by && document.getElementById(by)?.textContent) || '')
  }
  const all = (sel, n) => [...document.querySelectorAll(sel)].filter((e) => vis(e) && !e.closest('#__ev_overlay')).slice(0, n)
  const uniq = (rows) => [...new Set(rows)]
  const headings = uniq(all('h1, h2, h3, [role=heading]', 15).map((h) => `${h.tagName.toLowerCase()}: ${clean(h.innerText, 80)}`).filter((x) => !x.endsWith(': ')))
  const links = []
  for (const a of all('a[href], [role=link]', 80)) {
    const name = labelOf(a) || clean(a.innerText)
    if (!name) continue
    const href = (a.getAttribute('href') || '').replace(/^https?:\/\/[^/]+/, '')
    const t = tid(a)
    links.push({ target: t ? `testid=${t}` : `role=link[name=${q(name)}]`, href: href || '(script)' })
    if (links.length >= 40) break
  }
  const buttons = uniq(all('button, [role=button], input[type=submit], input[type=button]', 60).map((b) => {
    const name = labelOf(b) || clean(b.innerText || b.value || b.title)
    const t = tid(b)
    return t ? `testid=${t}${name ? `  (${name})` : ''}` : name ? `role=button[name=${q(name)}]` : ''
  }).filter(Boolean)).slice(0, 40)
  const fields = all('input:not([type=hidden]):not([type=submit]):not([type=button]), textarea, select, [contenteditable=true], [role=combobox]', 40).map((f) => {
    const label = labelOf(f)
    const t = tid(f)
    const ph = f.getAttribute('placeholder')
    const target = label ? `label=${label}` : ph ? `placeholder=${clean(ph)}` : t ? `testid=${t}` : f.name ? `[name=${q(f.name)}]` : f.id ? `#${f.id}` : f.tagName.toLowerCase()
    const kind = f.tagName === 'SELECT' ? 'native select' : f.getAttribute('role') === 'combobox' ? 'custom dropdown: click it, then click the option' : f.type || f.tagName.toLowerCase()
    const options = f.tagName === 'SELECT' ? [...f.options].slice(0, 8).map((o) => clean(o.text, 30)) : null
    return { target, kind, options }
  })
  const tables = all('table, [role=grid], [role=table]', 6).map((t, i) => {
    const cols = [...t.querySelectorAll('th, [role=columnheader]')].slice(0, 10).map((c) => clean(c.innerText, 24)).filter(Boolean)
    const rows = t.querySelectorAll('tbody tr, [role=row]').length
    const id = tid(t)
    return { target: id ? `testid=${id}` : i ? `table >> nth=${i}` : 'table', cols, rows }
  })
  const testids = uniq(all('[data-testid], [data-test-id], [data-test], [data-qa], [data-cy]', 200)
    .map((e) => `testid=${tid(e)}  <${e.tagName.toLowerCase()}> ${clean(e.innerText, 40)}`)).slice(0, 60)
  const dialogs = all('[role=dialog], [role=alertdialog], dialog[open]', 5).map((d) => clean(labelOf(d) || d.innerText, 80))
  return { title: clean(document.title, 80), headings, links, buttons, fields, tables, testids, dialogs }
}

// ---------------------------------------------------------------------------------------------

const suiteRuns = new Map()

async function recordCase(browser, sc) {
  const name = safeName(sc.id)
  const steps = sc.steps || []
  const base = sc.base_url || board.base_url || ''
  const apiBase = sc.api_base || board.api_base || base
  for (const dir of ['logs', 'shots']) {
    mkdirSync(join(outDir, dir), { recursive: true })
    for (const f of readdirSync(join(outDir, dir))) if (f.startsWith(name + '-')) rmSync(join(outDir, dir, f), { force: true })
  }

  // A suite's test runs go first, outside the video, so minutes of a test suite do not become
  // minutes of video; in flows and acceptance cases every command runs live (unless live: false).
  const runs = new Map()
  const preRun = (st) => st.action === 'run' && (kindOf(sc) === 'suite' ? st.live !== true : st.live === false)
  for (const [i, st] of steps.entries()) {
    if (!preRun(st)) continue
    const log = `logs/${name}-${String(i + 1).padStart(2, '0')}.log`
    console.log(`  running ${redact(st.command)} …`)
    runs.set(i, { log, ...(await runCommand(st, join(outDir, log))) })
  }

  const repos = repoInfo([...steps.map((s) => s.cwd), ...(board.repos || []), ...(sc.repos || [])])
  const environment = { base_url: base, browser: `Chromium ${browser.version()}`, viewport: `${W}×${H}`, repos }
  const envLine = repos.map((r) => `${r.name}@${r.commit}${r.uncommitted ? '+changes' : ''} (${r.branch})`).join(' · ')

  // The case's session: it starts signed in (a case about signing in sets "session": false).
  const sessName = sc.session === false || sc.storage_state ? null : sc.session || board.session || null
  let sess = null
  let sessionError = ''
  if (sessName) {
    try { sess = await ensureSession(browser, sessName) } catch (err) { sessionError = String(err?.message || err) }
  }

  const tmpVideo = join(outDir, '.rec-' + name)
  rmSync(tmpVideo, { recursive: true, force: true })
  const recordVideo = { dir: tmpVideo, size: { width: W, height: H } }
  const context = sess
    ? await sessionContext(browser, sess.cfg, sess.saved, { recordVideo })
    : await browser.newContext(contextOptions({ recordVideo, storageState: sc.storage_state || (sessName ? undefined : board.storage_state) || undefined }))
  const page = await context.newPage()
  const t0 = Date.now()
  page.setDefaultTimeout(STEP_TIMEOUT)

  // Browser console errors and failing requests: reported with the case, they often explain a failure.
  const issues = []
  const issue = (t) => { const s = redact(t).slice(0, 240); if (issues.length < 30 && !issues.includes(s)) issues.push(s) }
  page.on('console', (m) => { if (m.type() === 'error') issue(`console: ${m.text()}`) })
  page.on('pageerror', (e) => issue(`page error: ${e.message}`))
  page.on('response', (r) => { if (r.status() >= 500) issue(`HTTP ${r.status()} ${r.request().method()} ${pathOf(r.url())}`) })
  page.on('requestfailed', (r) => {
    const why = r.failure()?.errorText || ''
    if (!/ERR_ABORTED/.test(why)) issue(`request failed ${r.method()} ${pathOf(r.url())}: ${why}`)
  })

  const total = steps.length
  const parts = []
  let status = 'passed'
  const fail = (s) => { if (status !== 'failed') status = s }
  const at = () => Math.round((Date.now() - t0) / 100) / 10
  const shot = async (i) => {
    const p = `shots/${name}-${String(i + 1).padStart(2, '0')}.png`
    await page.screenshot({ path: join(outDir, p) }).catch(() => {})
    return p
  }

  // Title card: what is tested, under which conditions, and the steps to come.
  const gw = [['Given', sc.given], ['Expected', sc.expected]].filter(([, v]) => v)
  const agenda = steps.slice(0, 12).map((s, i) => `<div><span>${i + 1}.</span>${esc(stepCaption(s))}</div>`).join('') +
    (steps.length > 12 ? `<div><span></span>… ${steps.length - 12} more steps</div>` : '')
  const usesApp = base && steps.some((s) => !CARD_ACTIONS.includes(s.action))
  await page.setContent(cardPage(sc, 'running', KIND_LABEL[kindOf(sc)] || 'Test case', `<div class="big">${esc(redact(sc.title))}</div>
    ${gw.length ? `<div class="gw">${gw.map(([k, v]) => `<b>${k}</b><span>${esc(redact(v))}</span>`).join('')}</div>` : ''}
    <div class="chips">${(sc.covers || []).map((c) => `<span class="chip">covers ${esc(c)}</span>`).join('')}
      ${sc.blocked ? `<span class="chip bad">⚠ partly blocked: ${esc(redact(sc.blocked))}</span>` : ''}
      <span class="chip">${total} step${total === 1 ? '' : 's'}</span>${usesApp ? `<span class="chip">app ${esc(base)}</span>` : ''}
      ${sess ? `<span class="chip ok">signed in as ${esc(sess.cfg.label || sess.name)}</span>` : ''}${sessionError ? `<span class="chip bad">✗ sign-in as ${esc(sessName)} failed</span>` : ''}</div>
    <div class="kind" style="margin-top:6px">Steps</div><div class="agenda">${agenda}</div>`, 0, total, envLine))
  await page.waitForTimeout(Math.min(HOLD.title + steps.length * 300, 8000))

  // The case's background processes by name, and the one whose log the drawer shows.
  const procs = new Map()
  const ctx = { base, cursor: null, n: 0, show: null, screen: '', monitor: null }
  const conView = () => {
    const h = ctx.monitor && procs.get(ctx.monitor)
    return h ? { title: `${ctx.monitor} — live log`, html: termHTML({ lines: h.lines, max: 12 }) } : null
  }
  const draw = (o) => overlay(page, { ...o, console: conView() })
  // Holding on a screen keeps the drawer live.
  const hold = async (ms) => {
    if (!conView()) return page.waitForTimeout(ms)
    const end = Date.now() + ms
    while (Date.now() < end) {
      await page.evaluate((html) => { const e = document.getElementById('__ev_con_lines'); if (e) e.innerHTML = html }, conView().html).catch(() => {})
      await page.waitForTimeout(Math.min(400, Math.max(0, end - Date.now())))
    }
  }
  const procOf = (st) => {
    const h = procs.get(st.process)
    if (!h) throw new Error(`no process named ${st.process}: start it first`)
    return h
  }
  let browserBroken = ''
  if (sessionError) {
    // Without its session the case's screens are out of reach: its browser steps are not run.
    parts.push({ action: 'session', caption: `Sign in as ${sessName}`, status: 'failed', error: redact(sessionError).slice(0, 400) })
    fail('failed')
    browserBroken = `signing in as ${sessName} failed`
    await page.setContent(cardPage(sc, 'failed', 'Sign-in failed', `<div class="mid">Could not sign in as ${esc(sessName)}</div>
      <div class="sub bad">${esc(redact(sessionError))}</div>`, 0, total, envLine))
    await page.waitForTimeout(HOLD.card)
  }
  for (const [i, st] of steps.entries()) {
    const caption = st.caption || `${st.action} ${st.target || st.value || ''}`.trim()
    ctx.n = i + 1
    ctx.show = async (o, ms) => {
      await draw({ caseId: sc.id, n: i + 1, total, caption, note: st.note, ...o })
      await hold(ms)
    }
    if ('monitor' in st) ctx.monitor = st.monitor || null
    const part = { action: st.action, caption: stepCaption(st), at: at() }
    if (st.note) part.note = redact(st.note)
    // After a failed browser step the screen is not where the later browser steps expect it: they
    // are listed as not run, while test commands and API calls still play.
    if (browserBroken && !CARD_ACTIONS.includes(st.action)) {
      Object.assign(part, { status: 'skipped', error: `not run: ${browserBroken}` })
      delete part.at
      parts.push(part)
      continue
    }
    try {
      if (st.action === 'run' && !runs.has(i)) {
        // Live: the command typed into a terminal, its output streaming, then its exit code.
        const log = `logs/${name}-${String(i + 1).padStart(2, '0')}.log`
        const cmd = redact(st.command)
        const frame = await showCard(page, cardPage(sc, 'running', 'Terminal', terminalBody(st, `${basename(st.cwd || boardDir)} — terminal`), i + 1, total, envLine))
        await typeCommand(frame, cmd)
        const h = spawnProc(st, join(outDir, log))
        while (!h.done) {
          await termUpdate(frame, termHTML({ cmd, lines: h.lines, hl: st.highlight }), RUNNING(h))
          await Promise.race([h.exit, sleep(350)])
        }
        const ok = h.code === 0
        const counts = countTests(h.lines)
        Object.assign(part, { command: cmd, exit_code: h.code, seconds: h.seconds, log, status: ok ? 'passed' : 'failed' })
        if (counts) part.summary = countLine(counts)
        if (h.timedOut) part.error = `stopped after ${st.timeout_s || 480}s`
        else if (!ok) part.error = `exit code ${h.code}`
        if (!ok) fail('failed')
        const chips = [[ok ? '✓ exit code 0' : h.timedOut ? '✗ timed out' : `✗ exit code ${h.code}`, ok ? 'ok' : 'bad']]
        if (counts) chips.push([countLine(counts), counts.failed ? 'bad' : 'ok'])
        chips.push([`ran ${fmtSecs(h.seconds)}`], [`full log: ${log}`])
        await termUpdate(frame, termHTML({ cmd, lines: h.lines, hl: st.highlight }), ok ? ['✓ exit 0', '#34d399'] : ['✗ exit ' + (h.code ?? '—'), '#f87171'], chips)
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.run)
      } else if (st.action === 'start') {
        // A process of the system — server, worker, scheduler, consumer, mock receiver — started in
        // the background and shown until it is ready. It runs until `stop` or the end of the case.
        if (!st.name) throw new Error('start needs a name')
        const log = `logs/${name}-${safeName(st.name)}.log`
        const cmd = redact(st.command)
        const frame = await showCard(page, cardPage(sc, 'running', 'Process', terminalBody(st, `${st.name} — ${basename(st.cwd || boardDir)}`), i + 1, total, envLine))
        await typeCommand(frame, cmd)
        const h = spawnProc(st, join(outDir, log), true)
        procs.set(st.name, h)
        let ready = null
        try { ready = st.ready ? new RegExp(st.ready, 'i') : null } catch { throw new Error(`ready is not a valid pattern: ${st.ready}`) }
        const deadline = Date.now() + (ready ? (st.timeout_s || 120) * 1000 : (st.wait_s || 3) * 1000)
        let ok = !ready
        while (Date.now() < deadline && !h.done) {
          if (ready && h.lines.some((l) => ready.test(l))) { ok = true; break }
          await termUpdate(frame, termHTML({ cmd, lines: h.lines, hit: st.ready }), RUNNING(h))
          await sleep(350)
        }
        const secs = Math.round((Date.now() - h.started) / 1000)
        if (h.done) { ok = false; part.error = `${st.name} exited (code ${h.code}) before it was ready` }
        else if (!ok) part.error = `${st.name} was not ready within ${st.timeout_s || 120}s (waiting for /${st.ready}/)`
        h.mark = h.count
        Object.assign(part, { command: cmd, log, seconds: secs, status: ok ? 'passed' : 'failed', summary: ok ? `${st.name} running · ready in ${fmtSecs(secs)}` : 'not ready' })
        if (!ok) fail('failed')
        await termUpdate(frame, termHTML({ cmd, lines: h.lines, hit: st.ready }), ok ? ['● running — ready', '#34d399'] : ['✗ not ready', '#f87171'],
          [[ok ? `✓ ${st.name} is up` : `✗ ${part.error}`, ok ? 'ok' : 'bad'], [`log: ${log}`]])
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.step)
      } else if (st.action === 'watch' || st.action === 'expect_log') {
        // Watching a process's log, live: `watch` waits for a new line matching `until` (written
        // since the previous check); `expect_log` observes for `within_s` more seconds, then counts
        // the lines matching `pattern` in everything the process wrote (since: "check" — only
        // since the previous check) — e.g. the alert is sent exactly once and never retried.
        const h = procOf(st)
        const pattern = st.action === 'watch' ? st.until : st.pattern
        let re
        try { re = new RegExp(pattern, 'i') } catch { throw new Error(`not a valid pattern: ${pattern}`) }
        const frame = await showCard(page, cardPage(sc, 'running', 'Log', terminalBody(st, `${st.process} — live log`), i + 1, total, envLine))
        const from = st.action === 'expect_log' && st.since !== 'check' ? 0 : h.mark
        const t1 = Date.now()
        const window = (st.action === 'watch' ? st.timeout_s || 60 : st.within_s || 5) * 1000
        const hits = () => h.since(from).filter((l) => re.test(l)).length
        let seen = false
        while (Date.now() - t1 < window) {
          if (st.action === 'watch' && hits() > 0) { seen = true; break }
          if (h.done && st.action === 'watch') break
          await termUpdate(frame, termHTML({ lines: h.since(Math.max(0, h.mark - 6)), hit: pattern, max: 28 }),
            [`◉ ${st.action === 'watch' ? 'watching for' : 'counting'} /${pattern}/ · ${fmtSecs(Math.round((Date.now() - t1) / 1000))}`, '#60a5fa'])
          await sleep(350)
        }
        const n = hits()
        const secs = Math.round((Date.now() - t1) / 1000)
        let ok, want
        if (st.action === 'watch') {
          ok = seen || n > 0
          want = `“${pattern}” appears`
        } else {
          const exact = st.count != null ? Number(st.count) : null
          const min = st.min != null ? Number(st.min) : exact == null && st.max == null ? 1 : null
          const max = st.max != null ? Number(st.max) : null
          ok = (exact == null || n === exact) && (min == null || n >= min) && (max == null || n <= max)
          want = exact != null ? `exactly ${exact}` : [min != null ? `at least ${min}` : '', max != null ? `at most ${max}` : ''].filter(Boolean).join(' and ')
        }
        h.mark = h.count
        const summary = st.action === 'expect_log'
          ? `“${pattern}” × ${n}${from === 0 ? ' since start' : ''}, watched ${fmtSecs(secs)} more (expected ${want})`
          : `“${pattern}” seen after ${fmtSecs(secs)}`
        Object.assign(part, { log: `logs/${name}-${safeName(st.process)}.log`, status: ok ? 'passed' : 'failed', verified: ok, summary, seconds: secs })
        if (!ok) { part.error = st.action === 'watch' ? `“${pattern}” did not appear within ${fmtSecs(secs)}` : `“${pattern}” appeared ${n}×, expected ${want}`; fail('failed') }
        await termUpdate(frame, termHTML({ lines: st.action === 'expect_log' ? h.since(from) : h.since(Math.max(0, from - 6)), hit: pattern, max: 28 }),
          ok ? ['✓ verified', '#34d399'] : ['✗ not as expected', '#f87171'], [[ok ? `✓ ${summary}` : `✗ ${part.error}`, ok ? 'ok' : 'bad']])
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.check + 800)
      } else if (st.action === 'stop') {
        const h = procOf(st)
        const frame = await showCard(page, cardPage(sc, 'running', 'Process', terminalBody(st, `${st.process} — stopping`), i + 1, total, envLine))
        await h.stop()
        if (ctx.monitor === st.process) ctx.monitor = null
        Object.assign(part, { status: 'passed', log: `logs/${name}-${safeName(st.process)}.log`, summary: `${st.process} stopped after ${fmtSecs(h.seconds)}` })
        await termUpdate(frame, termHTML({ lines: h.lines, max: 28 }), ['■ stopped', '#94a3b8'], [[part.summary]])
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.step)
      } else if (st.action === 'run') {
        const r = runs.get(i)
        const ok = r.code === 0
        const counts = countTests(r.lines)
        Object.assign(part, { command: redact(st.command), exit_code: r.code, seconds: r.seconds, log: r.log, status: ok ? 'passed' : 'failed' })
        if (counts) part.summary = countLine(counts)
        if (r.timedOut) part.error = `stopped after ${st.timeout_s || 480}s`
        else if (!ok) part.error = `exit code ${r.code}`
        if (!ok) fail('failed')
        if (st.suite) {
          suiteRuns.set(st.suite, { repository: st.suite, command: redact(st.command), status: ok ? 'passed' : 'failed',
            ...(counts ? { passed: counts.passed, failed: counts.failed, ...(counts.skipped ? { skipped: counts.skipped } : {}) } : {}),
            seconds: r.seconds, log: r.log, case: sc.id })
        }
        const chips = [[ok ? '✓ exit code 0' : r.timedOut ? '✗ timed out' : `✗ exit code ${r.code}`, ok ? 'ok' : 'bad']]
        if (counts) chips.push([countLine(counts), counts.failed ? 'bad' : 'ok'])
        chips.push([`ran ${fmtSecs(r.seconds)}, started ${r.startedAt.slice(11)}`], [`full log: ${r.log}`])
        part.at = at()
        await showCard(page, cardPage(sc, ok ? 'passed' : 'failed', 'Test run', termBody(st, st.command, r.lines, chips), i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.run)
      } else if (st.action === 'log') {
        const file = isAbsolute(st.file) ? st.file : join(outDir, st.file)
        const lines = stripAnsi(readFileSync(file, 'utf8')).split('\n').map(redact)
        const counts = countTests(lines)
        // The status is the agent's claim; a log whose own summary reports failures never passes.
        const s = st.status === 'failed' || (counts && counts.failed > 0) ? 'failed' : 'passed'
        Object.assign(part, { log: st.file, status: s })
        if (counts) part.summary = countLine(counts)
        if (s === 'failed') fail('failed')
        const chips = [[s === 'passed' ? '✓ passed' : '✗ failed', s === 'passed' ? 'ok' : 'bad']]
        if (counts) chips.push([countLine(counts), counts.failed ? 'bad' : 'ok'])
        chips.push(['saved log, not re-run by the recorder'])
        await showCard(page, cardPage(sc, s, 'Test log', termBody(st, st.command || st.file, lines, chips), i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.log)
      } else if (st.action === 'api') {
        const log = `logs/${name}-${String(i + 1).padStart(2, '0')}-api.log`
        const r = await apiCall(st, apiBase, join(outDir, log), sess)
        const ok = r.checks.every(([, v]) => v)
        Object.assign(part, { method: r.method, url: r.path, http_status: r.status, summary: `HTTP ${r.status} · ${r.ms} ms`, log, status: ok ? 'passed' : 'failed' })
        if (!ok) { part.error = r.checks.filter(([, v]) => !v).map(([c]) => c).join('; '); fail('failed') }
        part.at = at()
        await showCard(page, cardPage(sc, ok ? 'passed' : 'failed', 'API call', apiBody(st, r), i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.api)
      } else if (st.action === 'card') {
        part.status = 'info'
        await showCard(page, cardPage(sc, 'running', st.kind || 'Note', `<div class="big">${esc(redact(st.caption))}</div>
          ${st.body ? `<div class="sub">${esc(redact(st.body))}</div>` : ''}${noteHTML(st.note)}`, i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.card)
      } else {
        await hideCard(page)
        const r = await browserStep(page, ctx, st)
        part.url = pathOf(page.url())
        const newScreen = part.url !== ctx.screen
        ctx.screen = part.url
        await draw({ caseId: sc.id, n: i + 1, total, caption, note: st.note, phase: r.phase, boxes: r.boxes, newScreen,
          cursor: r.phase === 'done' ? r.cursor : null })
        part.status = 'passed'
        if (r.phase === 'verified') part.verified = true
        part.shot = await shot(i)
        await hold(st.hold_ms || r.hold || (r.phase === 'verified' ? HOLD.check : HOLD.step))
        // `explain`: walk the viewer through the screen — its regions outlined, numbered and labelled.
        if (st.explain?.length) {
          const boxes = []
          for (const a of st.explain) {
            const b = await boxOf(await findTarget(page, a.target, 5000).catch(() => null))
            if (b) boxes.push({ ...b, label: a.label || '' })
          }
          if (boxes.length) {
            await draw({ caseId: sc.id, n: i + 1, total, phase: 'info', boxes, caption: st.screen || 'On this screen', note: st.screen_note })
            part.shot = await shot(i)
            await hold(Math.max(HOLD.annotate, 1500 * boxes.length))
          }
        }
      }
    } catch (err) {
      part.status = 'failed'
      part.error = redact(String(err?.message || err).split('\n')[0].slice(0, 300))
      fail(st.action === 'log' ? 'blocked' : 'failed')
      if (CARD_ACTIONS.includes(st.action) || !onApp(page)) {
        await showCard(page, cardPage(sc, 'failed', 'Step failed', `<div class="mid">${esc(part.caption)}</div>
          <div class="sub bad">${esc(part.error)}</div>${noteHTML(st.note)}`, i + 1, total, envLine))
      } else {
        await draw({ caseId: sc.id, n: i + 1, total, phase: 'failed', caption: `${caption} — ${part.error}`, note: st.note })
        part.url = pathOf(page.url())
      }
      part.shot = await shot(i)
      await page.waitForTimeout(HOLD.step + 1000)
      if (!CARD_ACTIONS.includes(st.action) && !sc.continue_on_failure) browserBroken = `step ${i + 1} failed`
    }
    parts.push(part)
  }

  // The case's processes stop with it.
  for (const h of procs.values()) await h.stop()

  // A flow or acceptance case proves the change end to end; one made only of unit-test runs does
  // not, and is reported Blocked with the reason.
  const unitOnly = kindOf(sc) !== 'suite' && steps.length > 0 && steps.every((x) => ['run', 'log', 'card'].includes(x.action)) &&
    steps.some((x) => UNIT_TEST.test(x.command || ''))
  if (unitOnly && !sc.blocked) sc.blocked = 'only unit-test runs — not end-to-end evidence: drive the flow through its UI, API, job or command instead'

  // A case that cannot fully run locally never passes: what is missing is stated on the result.
  if (sc.blocked && status === 'passed') status = 'blocked'

  // Result card: every step with its outcome, then what the browser reported.
  const icon = (p) => (p.status === 'passed' ? ['ok', '✓'] : p.status === 'info' ? ['dim', '•'] : p.status === 'skipped' ? ['dim', '–'] : ['bad', '✗'])
  const rows = parts.slice(0, 14).map((p, i) => {
    const [cls, ch] = icon(p)
    const t = p.summary || (p.seconds != null ? fmtSecs(p.seconds) : '')
    return `<div class="row"><b class="${cls}">${ch}</b><span>${i + 1}. ${esc(p.caption)}${p.error ? ` <span class="${cls}">— ${esc(p.error)}</span>` : ''}</span>${t ? `<span class="t">${esc(t)}</span>` : ''}</div>`
  }).join('') + (parts.length > 14 ? `<div class="row dim">… ${parts.length - 14} more steps</div>` : '')
  const checked = parts.filter((p) => p.status !== 'info')
  const passedN = checked.filter((p) => p.status === 'passed').length
  await page.setContent(cardPage(sc, status, 'Result', `<div class="big">${esc(redact(sc.title))}</div>
    ${sc.expected ? `<div class="gw"><b>Expected</b><span>${esc(redact(sc.expected))}</span></div>` : ''}
    ${sc.blocked ? `<div class="gw"><b class="bad">Blocked</b><span>${esc(redact(sc.blocked))}</span></div>` : ''}
    <div class="list">${rows}</div>
    <div class="chips"><span class="chip ${status === 'passed' ? 'ok' : status === 'blocked' ? '' : 'bad'}">${passedN} of ${checked.length} steps passed</span>
      ${issues.length ? `<span class="chip bad">${issues.length} browser error${issues.length === 1 ? '' : 's'} — listed with the case</span>` : ''}</div>`, 0, total, envLine))
  await page.screenshot({ path: join(outDir, name + '.png') })
  const resultAt = at()
  await page.waitForTimeout(HOLD.result)
  const duration = Math.round((Date.now() - t0) / 1000)
  const video = page.video()
  await context.close()

  let file = ''
  const raw = video ? await video.path().catch(() => '') : ''
  if (raw && existsSync(raw)) {
    const webm = join(outDir, name + '.webm')
    rmSync(join(outDir, name + '.mp4'), { force: true })
    renameSync(raw, webm)
    file = name + '.webm'
    if (toMP4(webm, join(outDir, name + '.mp4'))) {
      rmSync(webm, { force: true })
      file = name + '.mp4'
    }
  }
  rmSync(tmpVideo, { recursive: true, force: true })
  return {
    id: sc.id, kind: kindOf(sc), title: redact(sc.title), covers: sc.covers || [], given: redact(sc.given || ''), expected: redact(sc.expected || ''),
    notes: redact([sc.blocked ? `Blocked: ${sc.blocked}` : '', sc.notes || ''].filter(Boolean).join('\n')), status, video: file, poster: name + '.png', parts, result_at: resultAt, duration_s: duration,
    environment, issues, recorded_at: new Date().toISOString(),
  }
}

function writeManifest(results) {
  const path = join(outDir, 'evidence.json')
  let m = {}
  try { m = JSON.parse(readFileSync(path, 'utf8')) } catch {}
  const byId = new Map((m.cases || []).map((c) => [c.id, c]))
  for (const r of results) byId.set(r.id, r)
  const order = (board.cases || []).map((c) => c.id)
  const cases = [...byId.values()].sort((a, b) => {
    const ia = order.indexOf(a.id); const ib = order.indexOf(b.id)
    return (ia < 0 ? 1e9 : ia) - (ib < 0 ? 1e9 : ib)
  })
  // Suites: the ones already recorded or listed in the storyboard; a run recorded now, with its real
  // counts, replaces the entry of its repository.
  const suites = new Map()
  for (const s of [...(m.suites || []), ...(board.suites || [])]) if (!suites.get(s.repository)?.case) suites.set(s.repository, s)
  for (const s of suiteRuns.values()) suites.set(s.repository, s)
  const out = { ...m, title: board.title || m.title || '', ...(board.criteria ? { criteria: board.criteria.map((c) => ({ id: c.id, text: redact(c.text) })) } : {}), generated_by: 'test-evidence-video', updated_at: new Date().toISOString(), cases }
  if (suites.size) out.suites = [...suites.values()]
  writeFileSync(path, JSON.stringify(out, null, 2) + '\n')
}

// ---------------------------------------------------------------------------------------------
// The presentation (--summary): one video that presents the whole evidence to a reviewer — the
// verdict, the spec coverage, then a chapter per case with its key annotated steps, and the
// results. Built from the recorded cases (evidence.json, the step screenshots and result cards)
// and spec.json (the acceptance cases the orchestrator expects), so it never re-runs anything.
// ---------------------------------------------------------------------------------------------
const SLIDE_STYLE = `
  .slide { animation: __in .45s ease-out both; }
  @keyframes __in { from { opacity:0; transform:translateY(8px) } to { opacity:1; transform:none } }
  @keyframes __kb { from { transform:scale(1) } to { transform:scale(1.045) } }
  .shotwrap { flex:1; min-height:0; border-radius:12px; overflow:hidden; border:1px solid #334155; background:#020617; display:flex; align-items:center; justify-content:center; }
  .shotwrap img { max-width:100%; max-height:100%; object-fit:contain; animation: __kb 4s ease-out both; transform-origin:center; }
  .cap { display:flex; gap:12px; align-items:baseline; }
  .badge { font-size:12px; font-weight:800; letter-spacing:.08em; text-transform:uppercase; padding:3px 9px; border-radius:6px; white-space:nowrap; }
  .stats { display:grid; grid-template-columns:repeat(4,1fr); gap:12px; }
  .stat { border:1px solid #334155; border-radius:12px; padding:14px 18px; background:rgba(2,6,23,.4); }
  .stat b { display:block; font-size:30px; } .stat span { font-size:13px; color:#94a3b8; text-transform:uppercase; letter-spacing:.08em; }
  table { width:100%; border-collapse:collapse; font-size:15px; } td, th { text-align:left; padding:7px 10px; border-bottom:1px solid #1e293b; vertical-align:top; }
  th { font-size:11px; color:#64748b; text-transform:uppercase; letter-spacing:.1em; }
  .st { font-size:12px; font-weight:800; padding:3px 9px; border-radius:999px; white-space:nowrap; }
  .verdict { font-size:20px; font-weight:800; padding:10px 18px; border-radius:12px; align-self:flex-start; }
`
const ST = {
  passed: ['#d1fae5', '#047857', '✓ Passed'], proven: ['#d1fae5', '#047857', '✓ Proven'], failed: ['#fee2e2', '#b91c1c', '✗ Failed'],
  blocked: ['#fef3c7', '#b45309', '⚠ Blocked'], missing: ['#e2e8f0', '#475569', '○ No evidence'], unknown: ['#e2e8f0', '#475569', '— No result'],
}
const stPill = (k) => { const t = ST[k] || ST.unknown; return `<span class="st" style="background:${t[0]};color:${t[1]}">${t[2]}</span>` }
const BADGE = {
  verified: ['#10b981', '✓ Verified'], action: ['#f59e0b', 'Action'], run: ['#38bdf8', 'Test run'], api: ['#818cf8', 'API call'],
  info: ['#a78bfa', 'Look here'], process: ['#38bdf8', 'Process'], failed: ['#ef4444', '✗ Failed'], open: ['#f59e0b', 'Screen'], note: ['#94a3b8', 'Note'],
}
function partBadge(p) {
  if (p.status === 'failed') return BADGE.failed
  if (p.action === 'run' || p.action === 'log') return BADGE.run
  if (p.action === 'start' || p.action === 'stop') return BADGE.process
  if (p.action === 'watch' || p.action === 'expect_log') return BADGE.verified
  if (p.action === 'api') return BADGE.api
  if (p.action === 'annotate') return BADGE.info
  if (p.action === 'card') return BADGE.note
  if (p.verified) return BADGE.verified
  if (p.action === 'goto') return BADGE.open
  return BADGE.action
}

function slidePage(kicker, body, n, total) {
  const pct = total ? Math.round((n / total) * 100) : 0
  return `<!doctype html><html><head><meta charset="utf-8"><style>${STYLE}${SLIDE_STYLE}</style></head><body>
    <div class="prog"><i style="width:${pct}%"></i></div>
    <div class="top"><span class="tag">Evidence</span><span class="crumb">${esc(redact(board.title || ''))}</span><span class="stepno">${esc(kicker)}</span></div>
    <div class="main slide">${body}</div>
    <div class="foot"><span>${esc(board.label || 'Test evidence')}</span><span>${clock()}</span></div>
  </body></html>`
}

const dataURI = (rel) => {
  const p = rel && join(outDir, rel)
  if (!p || !existsSync(p)) return ''
  return 'data:image/png;base64,' + readFileSync(p).toString('base64')
}

// The steps worth showing in the presentation: every check, failure, test run, API call and
// annotation, plus — for a flow — the actions that move the journey on.
function highlights(c) {
  const parts = (c.parts || []).map((p, i) => ({ p, i })).filter((x) => x.p.shot)
  const kind = c.kind || 'acceptance'
  const key = (x) => x.p.status === 'failed' || x.p.verified || ['run', 'log', 'api', 'annotate', 'start'].includes(x.p.action)
  let pick = kind === 'flow' ? parts : parts.filter((x) => key(x) || x.p.action === 'goto')
  const max = Number(board.summary?.max_steps_per_case) || (kind === 'flow' ? 10 : 6)
  if (pick.length > max) {
    const must = pick.filter(key)
    pick = (must.length >= max ? must.slice(-max) : pick.filter((x) => key(x) || pick.indexOf(x) < max - must.length))
  }
  return pick
}

async function recordSummary(browser) {
  let m
  try { m = JSON.parse(readFileSync(join(outDir, 'evidence.json'), 'utf8')) } catch { throw new Error('no evidence.json yet: record the cases first') }
  let spec = null
  try { spec = JSON.parse(readFileSync(join(outDir, 'spec.json'), 'utf8')) } catch {}
  const order = { flow: 0, acceptance: 1, suite: 2 }
  const cases = [...(m.cases || [])].map((c) => ({ ...c, kind: c.kind || kindOf(c) }))
    .sort((a, b) => (order[a.kind] ?? 3) - (order[b.kind] ?? 3))
  if (!cases.length) throw new Error('evidence.json has no cases')

  // What the evidence must prove: the orchestrator's acceptance cases, else the listed criteria.
  const specCases = (spec?.cases?.length ? spec.cases : (m.criteria || []).map((c) => ({ id: c.id, title: c.text }))).map((sc) => {
    const ev = cases.filter((c) => c.kind !== 'suite' && (c.id === sc.id || (c.covers || []).some((v) => String(v).toLowerCase() === String(sc.id).toLowerCase())))
    const status = !ev.length ? 'missing' : ev.some((c) => c.status === 'failed') ? 'failed' : ev.some((c) => c.status === 'passed') ? 'proven' : 'blocked'
    return { ...sc, evidence: ev.map((c) => c.id), status }
  })
  const proven = specCases.filter((c) => c.status === 'proven').length
  const passed = cases.filter((c) => c.status === 'passed').length
  const suites = m.suites || []
  const bad = cases.filter((c) => c.status !== 'passed')
  const verdict = bad.length === 0 && proven === specCases.length
    ? ['#d1fae5', '#047857', specCases.length ? `✓ All ${specCases.length} acceptance cases proven` : `✓ All ${cases.length} cases passed`]
    : cases.some((c) => c.status === 'failed') || specCases.some((c) => c.status === 'failed')
      ? ['#fee2e2', '#b91c1c', `✗ Not ready — ${bad.length} case${bad.length === 1 ? '' : 's'} not passing`]
      : ['#fef3c7', '#b45309', `⚠ Partly proven — ${specCases.length - proven} acceptance case${specCases.length - proven === 1 ? '' : 's'} not proven`]

  // Build the slides first, so the progress bar knows the total.
  const slides = []
  const add = (kicker, body, hold, chapter) => slides.push({ kicker, body, hold, chapter })
  const flows = cases.filter((c) => c.kind === 'flow').length
  const acc = cases.filter((c) => c.kind === 'acceptance')
  add('Overview', `<div class="kind">Test evidence</div><div class="big" style="font-size:36px">${esc(redact(board.title || m.title || 'Test evidence'))}</div>
    <div class="verdict" style="background:${verdict[0]};color:${verdict[1]}">${esc(verdict[2])}</div>
    <div class="stats">
      <div class="stat"><span>Spec coverage</span><b>${specCases.length ? `${proven} / ${specCases.length}` : '—'}</b></div>
      <div class="stat"><span>End-to-end flows</span><b>${flows}</b></div>
      <div class="stat"><span>Acceptance cases</span><b>${acc.filter((c) => c.status === 'passed').length} / ${acc.length}</b></div>
      <div class="stat"><span>Full suites</span><b>${suites.length ? `${suites.filter((x) => x.status === 'passed').length} / ${suites.length}` : '—'}</b></div>
    </div>
    <div class="kind" style="margin-top:4px">In this video</div>
    <div class="agenda">${cases.slice(0, 9).map((c, i) => `<div><span>${i + 1}.</span>${esc(c.id)} — ${esc(c.title || '')}</div>`).join('')}
      ${cases.length > 9 ? `<div><span></span>… ${cases.length - 9} more</div>` : ''}</div>`, 7000, { id: 'overview', title: 'Overview' })

  for (let i = 0; i < specCases.length; i += 10) {
    const rows = specCases.slice(i, i + 10).map((c) => `<tr><td style="font-family:ui-monospace,monospace;white-space:nowrap">${esc(c.id)}</td>
      <td>${esc(redact(c.title || ''))}</td><td style="font-family:ui-monospace,monospace;color:#94a3b8">${esc(c.evidence.join(', ') || '—')}</td><td>${stPill(c.status)}</td></tr>`).join('')
    add('Spec coverage', `<div class="kind">What the spec asks — and what proves it</div>
      <div class="mid">${proven} of ${specCases.length} acceptance cases proven${spec?.source ? ` <span class="dim" style="font-size:15px;font-weight:400">· from ${esc(spec.source)}</span>` : ''}</div>
      <table><thead><tr><th>Case</th><th>What it proves</th><th>Evidence</th><th>Result</th></tr></thead><tbody>${rows}</tbody></table>`,
    5000 + Math.min(10, specCases.length - i) * 400, i === 0 ? { id: 'coverage', title: 'Spec coverage' } : null)
  }

  for (const c of cases) {
    const ps = (c.parts || []).filter((p) => p.status !== 'info')
    const label = { flow: 'End-to-end flow', acceptance: 'Acceptance case', suite: 'Regression suite' }[c.kind] || 'Test case'
    add(c.id, `<div class="kind">${esc(label)}</div>
      <div class="cap"><span class="tag" style="font-size:16px">${esc(c.id)}</span><div class="big">${esc(c.title || '')}</div></div>
      <div class="gw">${c.given ? `<b>Given</b><span>${esc(c.given)}</span>` : ''}${c.expected ? `<b>Expected</b><span>${esc(c.expected)}</span>` : ''}
        ${(c.covers || []).length ? `<b>Covers</b><span>${esc(c.covers.join(', '))}</span>` : ''}</div>
      <div class="chips">${stPill(c.status)}<span class="chip">${ps.filter((p) => p.status === 'passed').length} of ${ps.length} steps passed</span>
        ${c.duration_s ? `<span class="chip">full recording ${fmtSecs(c.duration_s)}: ${esc(c.video || '')}</span>` : ''}</div>
      ${c.notes ? `<div class="note">${esc(c.notes)}</div>` : ''}`, 3800, { id: c.id, title: `${c.id} — ${c.title || ''}` })
    for (const { p, i } of highlights(c)) {
      const [color, tag] = partBadge(p)
      const meta = [p.summary, p.error].filter(Boolean).join(' — ')
      add(c.id, `<div class="cap"><span class="badge" style="background:${color};color:#0f172a">${esc(tag)}</span>
          <div class="mid" style="font-size:20px">Step ${i + 1} of ${(c.parts || []).length} · ${esc(p.caption || p.action)}</div></div>
        ${p.note ? `<div class="note">${esc(p.note)}</div>` : ''}
        <div class="shotwrap"><img src="${dataURI(p.shot)}"></div>
        ${meta ? `<div class="sub" style="font-size:15px;${p.status === 'failed' ? 'color:#fca5a5' : ''}">${esc(meta)}</div>` : ''}`,
      p.status === 'failed' ? 4500 : 3400)
    }
    if (c.poster && existsSync(join(outDir, c.poster))) {
      add(c.id, `<div class="cap"><span class="badge" style="background:#a78bfa;color:#0f172a">Result</span><div class="mid" style="font-size:20px">${esc(c.id)} — ${esc(c.title || '')}</div></div>
        <div class="shotwrap"><img src="${dataURI(c.poster)}"></div>`, 3200)
    }
  }

  const rows = cases.map((c) => `<tr><td style="font-family:ui-monospace,monospace;white-space:nowrap">${esc(c.id)}</td><td style="color:#94a3b8">${esc(c.kind)}</td>
    <td>${esc(c.title || '')}</td><td>${stPill(c.status)}</td></tr>`).slice(0, 12).join('')
  const issues = [
    ...bad.map((c) => `${c.id}: ${(c.parts || []).find((p) => p.status === 'failed')?.error || c.notes || c.status}`),
    ...specCases.filter((c) => c.status === 'missing').map((c) => `${c.id}: no evidence recorded`),
    ...suites.filter((x) => x.status !== 'passed').map((x) => `suite ${x.repository}: ${x.failed ?? '?'} failing`),
  ]
  add('Summary', `<div class="kind">Summary</div>
    <div class="verdict" style="background:${verdict[0]};color:${verdict[1]}">${esc(verdict[2])}</div>
    <table><thead><tr><th>Case</th><th>Kind</th><th>Title</th><th>Result</th></tr></thead><tbody>${rows}</tbody></table>
    ${suites.length ? `<div class="chips">${suites.map((x) => `<span class="chip ${x.status === 'passed' ? 'ok' : 'bad'}">${esc(x.repository)}: ${x.passed ?? '?'} passed · ${x.failed ?? '?'} failed</span>`).join('')}</div>` : ''}
    ${issues.length ? `<div class="list">${issues.slice(0, 6).map((t) => `<div class="row"><b class="bad">!</b><span>${esc(redact(t))}</span></div>`).join('')}</div>` : ''}`,
  9000, { id: 'summary', title: 'Summary' })

  const name = 'SUMMARY'
  const tmpVideo = join(outDir, '.rec-' + name)
  rmSync(tmpVideo, { recursive: true, force: true })
  const context = await browser.newContext({ viewport: { width: W, height: H }, recordVideo: { dir: tmpVideo, size: { width: W, height: H } } })
  const page = await context.newPage()
  const t0 = Date.now()
  const chapters = []
  for (const [i, sl] of slides.entries()) {
    if (sl.chapter) chapters.push({ ...sl.chapter, at: Math.round((Date.now() - t0) / 100) / 10 })
    await page.setContent(slidePage(sl.kicker, sl.body, i + 1, slides.length))
    if (i === 0) await page.screenshot({ path: join(outDir, name + '.png') })
    await page.waitForTimeout(sl.hold)
  }
  const duration = Math.round((Date.now() - t0) / 1000)
  const video = page.video()
  await context.close()
  let file = ''
  const raw = video ? await video.path().catch(() => '') : ''
  if (raw && existsSync(raw)) {
    const webm = join(outDir, name + '.webm')
    rmSync(join(outDir, name + '.mp4'), { force: true })
    renameSync(raw, webm)
    file = name + '.webm'
    if (toMP4(webm, join(outDir, name + '.mp4'))) {
      rmSync(webm, { force: true })
      file = name + '.mp4'
    }
  }
  rmSync(tmpVideo, { recursive: true, force: true })
  m.summary = { video: file, poster: name + '.png', duration_s: duration, chapters, recorded_at: new Date().toISOString() }
  writeFileSync(join(outDir, 'evidence.json'), JSON.stringify(m, null, 2) + '\n')
  return m.summary
}

if (args.includes('--clear-sessions')) {
  rmSync(SESSION_DIR, { recursive: true, force: true })
  console.log(`saved sessions removed (${SESSION_DIR})`)
  if (!only.length && !args.includes('--summary') && !flagValues('--explore').length && !flagValues('--session').length) process.exit(0)
}

const chromium = await loadChromium()

// --explore / --session: sign in and look around, before (or between) recording.
const explore = flagValues('--explore')
const sessArg = flagValues('--session')[0]
if (explore.length || (sessArg && !only.length && !args.includes('--summary'))) {
  const browser = await chromium.launch({ headless: true }).catch(() => chromium.launch({ headless: true, channel: 'chrome' }))
  let code = 0
  try {
    if (explore.length) await explorePages(browser, explore, sessArg)
    else {
      const s = await ensureSession(browser, sessArg, { fresh: args.includes('--fresh') })
      console.log(`✓ session "${s.name}" ${s.reused ? 'still signed in (saved session reused)' : 'signed in'} — cases with "session": "${s.name}" start signed in` +
        `${Object.keys(s.apiHeaders).length || s.storage?.cookies?.length ? '; its api steps are signed too' : ''}`)
    }
  } catch (err) {
    console.error(`✗ ${redact(String(err?.message || err))}`)
    code = 1
  } finally {
    await browser.close()
  }
  process.exit(code)
}

const cases = (board.cases || []).filter((c) => !only.length || only.includes(c.id))
const summaryOnly = args.includes('--summary') && !only.length
if (!cases.length && !summaryOnly) {
  console.error(only.length ? `no case with id ${only.join(', ')} in the storyboard` : 'the storyboard has no cases')
  process.exit(2)
}
const browser = await chromium.launch({ headless: true }).catch(() => chromium.launch({ headless: true, channel: 'chrome' }))
const results = []
try {
  for (const sc of summaryOnly ? [] : cases) {
    const r = await recordCase(browser, sc)
    results.push(r)
    writeManifest(results)
    console.log(`${r.status === 'passed' ? '✓' : '✗'} ${r.id} ${r.status} → ${r.video || 'no video'} (result card ${r.poster}, a screenshot per step in shots/${safeName(r.id)}-NN.png)`)
    for (const p of r.parts) if (!['passed', 'info'].includes(p.status)) console.log(`    step ${p.action} "${p.caption}": ${p.error || p.status}${p.log ? ` (log ${p.log})` : ''}`)
    for (const i of r.issues) console.log(`    browser: ${i}`)
  }
  if (args.includes('--summary')) {
    const sm = await recordSummary(browser)
    console.log(`▶ presentation → ${sm.video} (${fmtSecs(sm.duration_s)}, ${sm.chapters.length} chapters)`)
  }
} finally {
  for (const h of [...allProcs]) await h.stop()
  await browser.close()
  for (const d of readdirSync(outDir)) if (d.startsWith('.rec-')) rmSync(join(outDir, d), { recursive: true, force: true })
}
process.exit(results.every((r) => r.status === 'passed') ? 0 : 1)
