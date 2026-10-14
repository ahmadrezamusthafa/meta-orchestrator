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
//   node record-evidence.mjs <storyboard.json> [--case <id> ...]
//
// playwright-core is resolved from this script's folders, then from EVIDENCE_PLAYWRIGHT_DIR.
import { readFileSync, writeFileSync, mkdirSync, existsSync, renameSync, rmSync, readdirSync, createWriteStream } from 'node:fs'
import { join, dirname, basename, resolve as resolvePath, isAbsolute } from 'node:path'
import { createRequire } from 'node:module'
import { spawn, spawnSync } from 'node:child_process'

const args = process.argv.slice(2)
const boardPath = args.find((a) => !a.startsWith('--'))
if (!boardPath) {
  console.error('usage: node record-evidence.mjs <storyboard.json> [--case <id> ...]')
  process.exit(2)
}
const only = []
for (let i = 0; i < args.length; i++) if (args[i] === '--case' && args[i + 1]) only.push(args[++i])

const board = JSON.parse(readFileSync(boardPath, 'utf8'))
const boardDir = dirname(resolvePath(boardPath))
const outDir = resolvePath(board.out_dir || boardDir)
mkdirSync(outDir, { recursive: true })
const W = board.viewport?.width || 1280
const H = board.viewport?.height || 720
const STEP_TIMEOUT = 15000
const HOLD = { title: 3000, pre: 1100, step: 2400, check: 2600, run: 5500, log: 5000, api: 6000, card: 4000, annotate: 4000, result: 5000, ...(board.hold_ms || {}) }
const CARD_ACTIONS = ['run', 'log', 'card', 'api']

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
const env = (v) => String(v ?? '').replace(/\$\{([A-Z0-9_]+)\}/g, (m, k) => process.env[k] ?? m)
const usesEnv = (v) => /\$\{[A-Z0-9_]+\}/.test(String(v ?? ''))
const pathOf = (u) => { try { const x = new URL(u); return x.pathname + (x.search ? '?…' : '') } catch { return String(u).split('?')[0] } }
const clock = () => new Date().toISOString().replace('T', ' ').slice(0, 19) + ' UTC'
const fmtSecs = (s) => (s >= 60 ? `${Math.floor(s / 60)}m ${s % 60}s` : `${s}s`)
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
      if (b.label || boxes.length > 1) {
        // The label goes beside the box when there is room, so it never covers what it points at.
        const tag = document.createElement('div')
        const side = b.x + b.width + 260 < window.innerWidth
        const where = side ? 'left:100%;margin-left:8px;top:50%;transform:translateY(-50%)'
          : b.y > 34 ? 'left:-3px;bottom:100%;margin-bottom:4px' : 'left:-3px;top:100%;margin-top:4px'
        tag.style.cssText = `position:absolute;${where};background:${color};color:#0f172a;box-shadow:0 2px 8px rgba(0,0,0,.3);` +
          'font-size:13px;font-weight:800;padding:3px 8px;border-radius:6px;white-space:nowrap;max-width:420px;overflow:hidden;text-overflow:ellipsis'
        tag.textContent = (boxes.length > 1 ? `${i + 1}  ` : '') + (b.label || '')
        ring.appendChild(tag)
      }
    })

    if (o.cursor) {
      const { x, y } = o.cursor
      if (o.click) el(`position:fixed;left:${x}px;top:${y}px;width:44px;height:44px;border-radius:50%;background:${color};animation:__ev_ripple .9s ease-out infinite`)
      const cur = el(`position:fixed;left:${x - 3}px;top:${y - 2}px;width:26px;height:30px`)
      cur.innerHTML = '<svg width="26" height="30" viewBox="0 0 26 30"><path d="M3 2 L3 24 L9 18.5 L13 27 L17 25.2 L13 16.8 L21 16.8 Z" fill="#fff" stroke="#0f172a" stroke-width="2" stroke-linejoin="round"/></svg>'
    }

    // The banner goes where it covers no outlined target.
    const low = boxes.some((b) => b.y + b.height > window.innerHeight * 0.6) && !boxes.some((b) => b.y < window.innerHeight * 0.35)
    const bar = el(`position:fixed;left:16px;right:16px;${low ? 'top:16px' : 'bottom:16px'};background:rgba(15,23,42,.95);color:#f8fafc;` +
      `border-left:6px solid ${color};border-radius:10px;padding:11px 16px;box-shadow:0 10px 30px rgba(0,0,0,.35)`)
    const head = document.createElement('div')
    head.style.cssText = 'font-size:17px;font-weight:600;line-height:1.35'
    const where = document.createElement('span')
    where.style.cssText = 'float:right;margin-left:16px;font-size:12px;font-weight:700;color:#94a3b8;letter-spacing:.04em;padding-top:3px'
    where.textContent = `${o.caseId} · Step ${o.n} of ${o.total}`
    head.appendChild(where)
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
    document.documentElement.appendChild(root)
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

// browserStep plays one browser step. An action on a target is shown twice: the target outlined
// with the cursor on it (what is about to happen), then the screen after it. Checks end Verified.
async function browserStep(page, ctx, st) {
  const url = (p) => (/^https?:\/\//.test(p) ? p : (ctx.base || '').replace(/\/$/, '') + (p.startsWith('/') ? p : '/' + p))
  const secret = st.secret || usesEnv(st.value)
  const act = async (loc, fn, click) => {
    const b = await boxOf(loc)
    ctx.cursor = center(b) || ctx.cursor
    await ctx.show({ phase: 'action', boxes: b ? [b] : [], cursor: ctx.cursor, click }, HOLD.pre)
    await fn(loc)
    await settle(page)
    const ab = await boxOf(await findTarget(page, st.target, 1200).catch(() => null))
    return { phase: 'done', boxes: ab ? [ab] : [], cursor: center(ab) || ctx.cursor }
  }
  const verified = async (loc, label) => ({ phase: 'verified', boxes: [await boxOf(loc)].filter(Boolean).map((b) => ({ ...b, label })) })
  switch (st.action) {
    case 'goto': {
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
// Test commands: run for real, output kept in logs/. They run before the case's video starts (unless
// `live`), so a suite of several minutes does not become minutes of a still frame; the video shows
// the real output, exit code and counts, how long it ran and when.
// ---------------------------------------------------------------------------------------------
function runCommand(st, logPath) {
  return new Promise((done) => {
    const started = Date.now()
    const startedAt = clock()
    const out = createWriteStream(logPath)
    out.write(`$ ${redact(st.command)}\n# cwd: ${st.cwd || boardDir}\n# started: ${startedAt}\n\n`)
    const child = spawn(st.command, { cwd: st.cwd || boardDir, shell: true, env: { ...process.env, CI: '1', FORCE_COLOR: '0', NO_COLOR: '1' } })
    // Whole lines only, redacted before they reach the log file or the screen.
    let lines = []
    let partial = ''
    const flush = (text) => {
      const got = text.split('\n').map((l) => redact(stripAnsi(l)))
      out.write(got.join('\n') + '\n')
      lines = lines.concat(got).slice(-4000)
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
    const limit = Math.min((st.timeout_s || 480) * 1000, 30 * 60 * 1000)
    const timer = setTimeout(() => child.kill('SIGTERM'), limit)
    child.on('close', (code, signal) => {
      clearTimeout(timer)
      if (partial) flush(partial)
      const seconds = Math.round((Date.now() - started) / 1000)
      out.end(`\n# exit code: ${signal ? `killed (${signal})` : code} after ${fmtSecs(seconds)}\n`)
      done({ code: signal ? null : code, timedOut: !!signal, seconds, lines, startedAt })
    })
  })
}

// API calls to the service running locally: the request (credentials come from the environment and
// are never shown), the response and the checks on it. The redacted exchange is saved in logs/.
async function apiCall(st, base, logPath) {
  const method = String(st.method || 'GET').toUpperCase()
  const raw = env(st.url || st.target)
  const target = /^https?:\/\//.test(raw) ? raw : String(base || '').replace(/\/$/, '') + (raw.startsWith('/') ? raw : '/' + raw)
  const headers = Object.fromEntries(Object.entries(st.headers || {}).map(([k, v]) => [k, env(v)]))
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

  // Test commands run first, outside the video (see runCommand).
  const runs = new Map()
  for (const [i, st] of steps.entries()) {
    if (st.action !== 'run' || st.live) continue
    const log = `logs/${name}-${String(i + 1).padStart(2, '0')}.log`
    console.log(`  running ${redact(st.command)} …`)
    runs.set(i, { log, ...(await runCommand(st, join(outDir, log))) })
  }

  const repos = repoInfo([...steps.map((s) => s.cwd), ...(board.repos || []), ...(sc.repos || [])])
  const environment = { base_url: base, browser: `Chromium ${browser.version()}`, viewport: `${W}×${H}`, repos }
  const envLine = repos.map((r) => `${r.name}@${r.commit}${r.uncommitted ? '+changes' : ''} (${r.branch})`).join(' · ')

  const tmpVideo = join(outDir, '.rec-' + name)
  rmSync(tmpVideo, { recursive: true, force: true })
  const context = await browser.newContext({
    viewport: { width: W, height: H },
    recordVideo: { dir: tmpVideo, size: { width: W, height: H } },
    storageState: sc.storage_state || board.storage_state || undefined,
    ignoreHTTPSErrors: true,
    locale: board.locale || undefined,
    timezoneId: board.timezone || undefined,
  })
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
  await page.setContent(cardPage(sc, 'running', 'Test case', `<div class="big">${esc(redact(sc.title))}</div>
    ${gw.length ? `<div class="gw">${gw.map(([k, v]) => `<b>${k}</b><span>${esc(redact(v))}</span>`).join('')}</div>` : ''}
    <div class="chips">${(sc.covers || []).map((c) => `<span class="chip">covers ${esc(c)}</span>`).join('')}
      <span class="chip">${total} step${total === 1 ? '' : 's'}</span>${usesApp ? `<span class="chip">app ${esc(base)}</span>` : ''}</div>
    <div class="kind" style="margin-top:6px">Steps</div><div class="agenda">${agenda}</div>`, 0, total, envLine))
  await page.waitForTimeout(Math.min(HOLD.title + steps.length * 300, 8000))

  const ctx = { base, cursor: null, n: 0, show: null }
  let browserBroken = ''
  for (const [i, st] of steps.entries()) {
    const caption = st.caption || `${st.action} ${st.target || st.value || ''}`.trim()
    ctx.n = i + 1
    ctx.show = async (o, hold) => {
      await overlay(page, { caseId: sc.id, n: i + 1, total, caption, note: st.note, ...o })
      await page.waitForTimeout(hold)
    }
    const part = { action: st.action, caption: stepCaption(st), at: at() }
    if (st.note) part.note = redact(st.note)
    // After a failed browser step the screen is not where the later browser steps expect it: they
    // are listed as not run, while test commands and API calls still play.
    if (browserBroken && !CARD_ACTIONS.includes(st.action)) {
      Object.assign(part, { status: 'skipped', error: `not run: step ${browserBroken} failed` })
      delete part.at
      parts.push(part)
      continue
    }
    try {
      if (st.action === 'run') {
        let r = runs.get(i)
        if (!r) {
          const log = `logs/${name}-${String(i + 1).padStart(2, '0')}.log`
          await page.setContent(cardPage(sc, 'running', 'Test run', `<div class="mid">${esc(redact(st.caption || 'Test command'))}</div>
            <div class="term"><div class="cmd">$ ${esc(redact(st.command))}</div><div class="out dim">running…</div></div>`, i + 1, total, envLine))
          r = { log, ...(await runCommand(st, join(outDir, log))) }
        }
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
        await page.setContent(cardPage(sc, ok ? 'passed' : 'failed', 'Test run', termBody(st, st.command, r.lines, chips), i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.run)
      } else if (st.action === 'log') {
        const file = isAbsolute(st.file) ? st.file : join(outDir, st.file)
        const lines = stripAnsi(readFileSync(file, 'utf8')).split('\n').map(redact)
        const s = st.status === 'failed' ? 'failed' : 'passed'
        const counts = countTests(lines)
        Object.assign(part, { log: st.file, status: s })
        if (counts) part.summary = countLine(counts)
        if (s === 'failed') fail('failed')
        const chips = [[s === 'passed' ? '✓ passed' : '✗ failed', s === 'passed' ? 'ok' : 'bad']]
        if (counts) chips.push([countLine(counts), counts.failed ? 'bad' : 'ok'])
        chips.push(['saved log, not re-run by the recorder'])
        await page.setContent(cardPage(sc, s, 'Test log', termBody(st, st.command || st.file, lines, chips), i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.log)
      } else if (st.action === 'api') {
        const log = `logs/${name}-${String(i + 1).padStart(2, '0')}-api.log`
        const r = await apiCall(st, apiBase, join(outDir, log))
        const ok = r.checks.every(([, v]) => v)
        Object.assign(part, { method: r.method, url: r.path, http_status: r.status, summary: `HTTP ${r.status} · ${r.ms} ms`, log, status: ok ? 'passed' : 'failed' })
        if (!ok) { part.error = r.checks.filter(([, v]) => !v).map(([c]) => c).join('; '); fail('failed') }
        part.at = at()
        await page.setContent(cardPage(sc, ok ? 'passed' : 'failed', 'API call', apiBody(st, r), i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.api)
      } else if (st.action === 'card') {
        part.status = 'info'
        await page.setContent(cardPage(sc, 'running', st.kind || 'Note', `<div class="big">${esc(redact(st.caption))}</div>
          ${st.body ? `<div class="sub">${esc(redact(st.body))}</div>` : ''}${noteHTML(st.note)}`, i + 1, total, envLine))
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || HOLD.card)
      } else {
        const r = await browserStep(page, ctx, st)
        await overlay(page, { caseId: sc.id, n: i + 1, total, caption, note: st.note, phase: r.phase, boxes: r.boxes, cursor: r.phase === 'done' ? r.cursor : null })
        part.status = 'passed'
        if (r.phase === 'verified') part.verified = true
        part.url = pathOf(page.url())
        part.shot = await shot(i)
        await page.waitForTimeout(st.hold_ms || r.hold || (r.phase === 'verified' ? HOLD.check : HOLD.step))
      }
    } catch (err) {
      part.status = 'failed'
      part.error = redact(String(err?.message || err).split('\n')[0].slice(0, 300))
      fail(st.action === 'log' ? 'blocked' : 'failed')
      if (CARD_ACTIONS.includes(st.action) || page.url() === 'about:blank' || page.url().startsWith('data:')) {
        await page.setContent(cardPage(sc, 'failed', 'Step failed', `<div class="mid">${esc(part.caption)}</div>
          <div class="sub bad">${esc(part.error)}</div>${noteHTML(st.note)}`, i + 1, total, envLine))
      } else {
        await overlay(page, { caseId: sc.id, n: i + 1, total, phase: 'failed', caption: `${caption} — ${part.error}`, note: st.note })
        part.url = pathOf(page.url())
      }
      part.shot = await shot(i)
      await page.waitForTimeout(HOLD.step + 1000)
      if (!CARD_ACTIONS.includes(st.action) && !sc.continue_on_failure) browserBroken = String(i + 1)
    }
    parts.push(part)
  }

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
    <div class="list">${rows}</div>
    <div class="chips"><span class="chip ${status === 'passed' ? 'ok' : 'bad'}">${passedN} of ${checked.length} steps passed</span>
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
    id: sc.id, title: redact(sc.title), covers: sc.covers || [], given: redact(sc.given || ''), expected: redact(sc.expected || ''),
    notes: redact(sc.notes || ''), status, video: file, poster: name + '.png', parts, result_at: resultAt, duration_s: duration,
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
  const out = { ...m, title: board.title || m.title || '', generated_by: 'test-evidence-video', updated_at: new Date().toISOString(), cases }
  if (suites.size) out.suites = [...suites.values()]
  writeFileSync(path, JSON.stringify(out, null, 2) + '\n')
}

const chromium = await loadChromium()
const cases = (board.cases || []).filter((c) => !only.length || only.includes(c.id))
if (!cases.length) {
  console.error(only.length ? `no case with id ${only.join(', ')} in the storyboard` : 'the storyboard has no cases')
  process.exit(2)
}
const browser = await chromium.launch({ headless: true }).catch(() => chromium.launch({ headless: true, channel: 'chrome' }))
const results = []
try {
  for (const sc of cases) {
    const r = await recordCase(browser, sc)
    results.push(r)
    writeManifest(results)
    console.log(`${r.status === 'passed' ? '✓' : '✗'} ${r.id} ${r.status} → ${r.video || 'no video'} (result card ${r.poster}, a screenshot per step in shots/${safeName(r.id)}-NN.png)`)
    for (const p of r.parts) if (!['passed', 'info'].includes(p.status)) console.log(`    step ${p.action} "${p.caption}": ${p.error || p.status}${p.log ? ` (log ${p.log})` : ''}`)
    for (const i of r.issues) console.log(`    browser: ${i}`)
  }
} finally {
  await browser.close()
  for (const d of readdirSync(outDir)) if (d.startsWith('.rec-')) rmSync(join(outDir, d), { recursive: true, force: true })
}
process.exit(results.every((r) => r.status === 'passed') ? 0 : 1)
