// Test evidence recorder: turns a storyboard of test cases into one narrated video per case. Each
// case opens with a title card, then plays its parts in order — test commands actually run here
// (real exit code and output), saved test logs, browser steps on the running app with a caption
// banner and the target highlighted — and ends with a result card. Results are merged into
// evidence.json next to the videos.
//
//   node record-evidence.mjs <storyboard.json> [--case <id> ...]
//
// playwright-core is resolved from this script's folders, then from EVIDENCE_PLAYWRIGHT_DIR.
import { readFileSync, writeFileSync, mkdirSync, existsSync, renameSync, rmSync, readdirSync, createWriteStream } from 'node:fs'
import { join, dirname, resolve as resolvePath, isAbsolute } from 'node:path'
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
const HOLD = { title: 3500, step: 2500, run: 5000, log: 5000, card: 4000, result: 4000, ...(board.hold_ms || {}) }

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
  [/\b((?:api[_-]?key|token|secret|password|passwd|authorization|bearer)["']?\s*[:=]\s*["']?)[^\s"',;]+/gi, '$1«redacted»'],
  [/\bBearer\s+[A-Za-z0-9._~+/=-]{8,}/g, 'Bearer «redacted»'],
  [/\b(?:sk|pk|ghp|gho|xox[abp])[-_][A-Za-z0-9_-]{10,}\b/g, '«redacted»'],
  [/\b[a-z][a-z0-9+.-]*:\/\/[^\s:/@]+:[^\s@/]+@/gi, '«redacted-url»@'],
  [/\b\d{13,19}\b/g, '«number»'], // card, NIK and account numbers
  [/(?:\+62|\b08)\d{8,12}\b/g, '«phone»'],
]
const redact = (t) => REDACT.reduce((s, [re, to]) => s.replace(re, to), String(t ?? ''))
const esc = (t) => String(t ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))
// eslint-disable-next-line no-control-regex
const stripAnsi = (t) => String(t ?? '').replace(/\x1b\[[0-9;?]*[ -/]*[@-~]/g, '')
const safeName = (id) => String(id).replace(/[^A-Za-z0-9._-]+/g, '_').slice(0, 80) || 'case'

// ---------------------------------------------------------------------------------------------
// Cards: full-screen pages drawn into the recorded tab between browser steps.
// ---------------------------------------------------------------------------------------------
const STYLE = `
  * { box-sizing:border-box; margin:0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; }
  body { width:${W}px; height:${H}px; overflow:hidden; background:linear-gradient(135deg,#0f172a,#1e293b); color:#f8fafc;
         display:flex; flex-direction:column; }
  .top { display:flex; align-items:center; gap:14px; padding:18px 28px; border-bottom:1px solid #334155; }
  .tag { background:#7c3aed; color:#fff; font-weight:800; font-size:14px; padding:5px 11px; border-radius:7px; white-space:nowrap; }
  .crumb { flex:1; min-width:0; font-size:14px; color:#94a3b8; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
  .pill { font-size:14px; font-weight:800; padding:6px 14px; border-radius:999px; white-space:nowrap; }
  .main { flex:1; min-height:0; padding:28px 40px; display:flex; flex-direction:column; gap:16px; }
  .kind { font-size:13px; font-weight:800; text-transform:uppercase; letter-spacing:.1em; color:#a78bfa; }
  .big { font-size:30px; font-weight:700; line-height:1.3; }
  .sub { font-size:18px; color:#cbd5e1; line-height:1.5; white-space:pre-wrap; }
  .chips { display:flex; flex-wrap:wrap; gap:8px; }
  .chip { font-size:14px; color:#e2e8f0; background:#334155; border-radius:999px; padding:6px 12px; }
  .term { flex:1; min-height:0; background:#020617; border:1px solid #334155; border-radius:12px; padding:14px 18px; overflow:hidden;
          font:13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; color:#cbd5e1; white-space:pre-wrap; word-break:break-word;
          display:flex; flex-direction:column; justify-content:flex-end; }
  .cmd { color:#f8fafc; font-weight:700; margin-bottom:6px; }
  .hl { color:#fde68a; } .ok { color:#6ee7b7; } .bad { color:#fca5a5; }
  .list { display:flex; flex-direction:column; gap:10px; }
  .row { display:flex; gap:12px; align-items:baseline; font-size:17px; color:#e2e8f0; }
  .row b { min-width:28px; }
  .foot { padding:10px 28px; font-size:12px; color:#64748b; border-top:1px solid #334155; display:flex; gap:18px; }
`
const TONE = {
  passed: ['#047857', '#d1fae5', '✓ Passed'], failed: ['#b91c1c', '#fee2e2', '✗ Failed'], blocked: ['#b45309', '#fef3c7', '⚠ Blocked'],
  running: ['#1d4ed8', '#dbeafe', '● Recording'],
}
function cardPage(sc, status, kind, body) {
  const t = TONE[status] || TONE.running
  return `<!doctype html><html><head><meta charset="utf-8"><style>${STYLE}</style></head><body>
    <div class="top"><span class="tag">${esc(sc.id)}</span><span class="crumb">${esc(redact(board.title || ''))} › ${esc(redact(sc.title))}</span>
      <span class="pill" style="background:${t[1]};color:${t[0]}">${t[2]}</span></div>
    <div class="main"><div class="kind">${esc(kind)}</div>${body}</div>
    <div class="foot"><span>${esc(board.label || 'Test evidence')}</span><span>${new Date().toISOString().replace('T', ' ').slice(0, 19)} UTC</span></div>
  </body></html>`
}

function termBody(caption, command, lines, footer, highlight) {
  const hl = highlight ? new RegExp(highlight, 'i') : null
  const tail = lines.slice(-34).map((l) => {
    const cls = /\b(fail(ed|ure)?|error|panic)\b/i.test(l) ? 'bad' : /\b(pass(ed)?|ok|success)\b/i.test(l) ? 'ok' : hl && hl.test(l) ? 'hl' : ''
    return cls ? `<span class="${cls}">${esc(l)}</span>` : esc(l)
  }).join('\n')
  return `<div class="big" style="font-size:22px">${esc(redact(caption))}</div>
    <div class="term"><div class="cmd">$ ${esc(redact(command))}</div>${tail}</div>
    <div class="chips">${footer.map((f) => `<span class="chip">${esc(f)}</span>`).join('')}</div>`
}

// ---------------------------------------------------------------------------------------------
// Browser steps: the real app, with a caption banner and the target outlined. The overlay is plain
// DOM with pointer-events off, re-drawn after every step because a navigation removes it.
// ---------------------------------------------------------------------------------------------
async function overlay(page, { caption, n, total, box, status }) {
  await page.evaluate(({ caption, n, total, box, status }) => {
    document.getElementById('__ev_overlay')?.remove()
    const root = document.createElement('div')
    root.id = '__ev_overlay'
    root.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:2147483647;font-family:system-ui,sans-serif'
    const color = status === 'failed' ? '#dc2626' : '#f59e0b'
    if (box) {
      const ring = document.createElement('div')
      ring.style.cssText = `position:fixed;left:${box.x - 6}px;top:${box.y - 6}px;width:${box.width + 12}px;height:${box.height + 12}px;` +
        `border:3px solid ${color};border-radius:10px;box-shadow:0 0 0 9999px rgba(15,23,42,.22)`
      root.appendChild(ring)
    }
    const bar = document.createElement('div')
    const atTop = box && box.y > window.innerHeight * 0.62
    bar.style.cssText = `position:fixed;left:16px;right:16px;${atTop ? 'top' : 'bottom'}:16px;background:rgba(15,23,42,.94);color:#f8fafc;` +
      `border-left:6px solid ${color};border-radius:10px;padding:12px 16px;font-size:17px;font-weight:600;box-shadow:0 10px 30px rgba(0,0,0,.35)`
    const step = document.createElement('span')
    step.style.cssText = `color:${color};font-size:12px;font-weight:800;letter-spacing:.08em;text-transform:uppercase;margin-right:10px`
    step.textContent = `Step ${n} of ${total}`
    bar.appendChild(step)
    bar.appendChild(document.createTextNode(caption))
    root.appendChild(bar)
    document.documentElement.appendChild(root)
  }, { caption: redact(caption), n, total, box, status }).catch(() => {})
}

// Blurs every email address, NIK or card number on the screen, so a test fixture that resembles
// real personal data never ends up readable in the video. Only the matched text is covered (a blur
// box in the overlay), so the sentence around it — often the proof of the case — stays readable.
async function maskPII(page) {
  await page.evaluate(() => {
    const root = document.getElementById('__ev_overlay')
    if (!root) return
    const re = /[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}|\b\d{13,19}\b/g
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
    if (Date.now() > deadline) throw new Error(`could not find ${target} on the screen within ${timeout / 1000}s`)
    await page.waitForTimeout(300)
  }
}

async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
  await page.waitForFunction(() => document.body && document.body.innerText.trim().length > 0, null, { timeout: 5000 }).catch(() => {})
}

const env = (v) => String(v ?? '').replace(/\$\{([A-Z0-9_]+)\}/g, (m, k) => process.env[k] ?? m)

async function browserStep(page, base, st) {
  const url = (p) => (/^https?:\/\//.test(p) ? p : (base || '').replace(/\/$/, '') + (p.startsWith('/') ? p : '/' + p))
  switch (st.action) {
    case 'goto': {
      const res = await page.goto(url(env(st.target)), { waitUntil: 'domcontentloaded', timeout: 30000 })
      if (res && res.status() >= 400) throw new Error(`HTTP ${res.status()} at ${st.target}`)
      break
    }
    case 'click': await (await findTarget(page, st.target)).click({ timeout: 5000 }); break
    case 'fill': await (await findTarget(page, st.target)).fill(env(st.value), { timeout: 5000 }); break
    case 'select': {
      const loc = await findTarget(page, st.target)
      await loc.selectOption({ label: env(st.value) }, { timeout: 5000 }).catch(() => loc.selectOption(env(st.value), { timeout: 5000 }))
      break
    }
    case 'check': await (await findTarget(page, st.target)).check({ timeout: 5000 }); break
    case 'press': await page.keyboard.press(st.value || 'Enter'); break
    case 'wait':
      if (st.target) await findTarget(page, st.target, 20000)
      else await page.waitForTimeout(Math.min(parseInt(st.value || '1000', 10) || 1000, 10000))
      break
    case 'expect_text': {
      const want = env(st.value)
      const deadline = Date.now() + 20000
      while (!(await page.getByText(want, { exact: false }).first().isVisible().catch(() => false))) {
        if (Date.now() > deadline) throw new Error(`expected text "${want}" did not appear`)
        await page.waitForTimeout(300)
      }
      return page.getByText(want, { exact: false }).first()
    }
    default: throw new Error(`unknown action ${st.action}`)
  }
  await settle(page)
  return st.target && st.action !== 'goto' && st.action !== 'wait' ? await findTarget(page, st.target, 1500).catch(() => null) : null
}

// ---------------------------------------------------------------------------------------------
// Test commands: run for real, output kept in logs/, shown as a terminal card with its exit code.
// ---------------------------------------------------------------------------------------------
function runCommand(st, logPath) {
  return new Promise((done) => {
    const started = Date.now()
    const out = createWriteStream(logPath)
    const child = spawn(st.command, { cwd: st.cwd || boardDir, shell: true, env: { ...process.env, CI: '1', FORCE_COLOR: '0' } })
    // Whole lines only, redacted before they reach the log file or the screen.
    let lines = []
    let partial = ''
    const flush = (text) => {
      const done = text.split('\n').map((l) => redact(stripAnsi(l)))
      out.write(done.join('\n') + '\n')
      lines = lines.concat(done).slice(-400)
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
      out.end()
      done({ code: signal ? null : code, timedOut: !!signal, seconds: Math.round((Date.now() - started) / 1000), lines })
    })
  })
}

function toMP4(webm, mp4) {
  if (spawnSync('ffmpeg', ['-version'], { stdio: 'ignore' }).status !== 0) return false
  const r = spawnSync('ffmpeg', ['-y', '-loglevel', 'error', '-i', webm, '-c:v', 'libx264', '-pix_fmt', 'yuv420p', '-movflags', '+faststart', mp4])
  return r.status === 0
}

// ---------------------------------------------------------------------------------------------

async function recordCase(browser, sc) {
  const name = safeName(sc.id)
  const tmpVideo = join(outDir, '.rec-' + name)
  rmSync(tmpVideo, { recursive: true, force: true })
  const context = await browser.newContext({
    viewport: { width: W, height: H },
    recordVideo: { dir: tmpVideo, size: { width: W, height: H } },
    storageState: sc.storage_state || board.storage_state || undefined,
    ignoreHTTPSErrors: true,
  })
  const page = await context.newPage()
  page.setDefaultTimeout(STEP_TIMEOUT)
  const base = sc.base_url || board.base_url || ''
  const steps = sc.steps || []
  const parts = []
  let status = 'passed'
  const fail = (s) => { if (status !== 'failed') status = s }

  await page.setContent(cardPage(sc, 'running', 'Test case', `<div class="big">${esc(redact(sc.title))}</div>
    ${sc.expected ? `<div class="sub">Expected: ${esc(redact(sc.expected))}</div>` : ''}
    <div class="chips">${(sc.covers || []).map((c) => `<span class="chip">covers ${esc(c)}</span>`).join('')}
      <span class="chip">${steps.length} part${steps.length === 1 ? '' : 's'}</span></div>`))
  await page.waitForTimeout(HOLD.title)

  const browserTotal = steps.filter((s) => !['run', 'log', 'card'].includes(s.action)).length
  let browserN = 0
  for (const st of steps) {
    const part = { action: st.action, caption: redact(st.caption || st.command || st.target || '') }
    try {
      if (st.action === 'run') {
        mkdirSync(join(outDir, 'logs'), { recursive: true })
        const log = `logs/${name}-${parts.filter((p) => p.action === 'run').length + 1}.log`
        const r = await runCommand(st, join(outDir, log))
        const ok = r.code === 0
        Object.assign(part, { command: redact(st.command), exit_code: r.code, seconds: r.seconds, log, status: ok ? 'passed' : 'failed' })
        if (r.timedOut) part.error = `stopped after ${st.timeout_s || 480}s`
        if (!ok) fail('failed')
        await page.setContent(cardPage(sc, ok ? 'passed' : 'failed', 'Test run', termBody(st.caption || 'Test command', st.command,
          r.lines, [ok ? '✓ exit code 0' : r.timedOut ? '✗ timed out' : `✗ exit code ${r.code}`, `${r.seconds}s`, `full log: ${log}`], st.highlight)))
        await page.waitForTimeout(HOLD.run)
      } else if (st.action === 'log') {
        const file = isAbsolute(st.file) ? st.file : join(outDir, st.file)
        const lines = stripAnsi(readFileSync(file, 'utf8')).split('\n').map(redact)
        const s = st.status === 'failed' ? 'failed' : 'passed'
        Object.assign(part, { log: st.file, status: s })
        if (s === 'failed') fail('failed')
        await page.setContent(cardPage(sc, s, 'Test log', termBody(st.caption || 'Test output', st.command || st.file, lines, [s === 'passed' ? '✓ passed' : '✗ failed'], st.highlight)))
        await page.waitForTimeout(HOLD.log)
      } else if (st.action === 'card') {
        await page.setContent(cardPage(sc, 'running', st.kind || 'Note', `<div class="big">${esc(redact(st.caption))}</div>${st.body ? `<div class="sub">${esc(redact(st.body))}</div>` : ''}`))
        await page.waitForTimeout(st.hold_ms || HOLD.card)
      } else {
        browserN++
        const target = await browserStep(page, base, st)
        const box = target ? await target.boundingBox().catch(() => null) : null
        await overlay(page, { caption: st.caption || `${st.action} ${st.target || st.value || ''}`, n: browserN, total: browserTotal, box, status: 'passed' })
        await maskPII(page)
        part.status = 'passed'
        part.url = page.url().split('?')[0]
        await page.waitForTimeout(st.hold_ms || HOLD.step)
      }
    } catch (err) {
      part.status = 'failed'
      part.error = redact(String(err?.message || err).split('\n')[0].slice(0, 300))
      fail(st.action === 'log' ? 'blocked' : 'failed')
      if (page.url() !== 'about:blank' && !page.url().startsWith('data:')) {
        await overlay(page, { caption: `${st.caption || st.action} — ${part.error}`, n: browserN, total: browserTotal, status: 'failed' })
        await maskPII(page)
        await page.waitForTimeout(HOLD.step)
      }
    }
    parts.push(part)
  }

  const rows = parts.map((p, i) => `<div class="row"><b class="${p.status === 'passed' ? 'ok' : 'bad'}">${p.status === 'passed' ? '✓' : '✗'}</b>
    <span>${i + 1}. ${esc(p.caption)}${p.error ? ` <span class="bad">— ${esc(p.error)}</span>` : ''}</span></div>`).join('')
  await page.setContent(cardPage(sc, status, 'Result', `<div class="big">${esc(redact(sc.title))}</div><div class="list">${rows}</div>`))
  await page.waitForTimeout(HOLD.result)
  await page.screenshot({ path: join(outDir, name + '.png') })
  const video = page.video()
  await context.close()

  let file = ''
  const raw = video ? await video.path().catch(() => '') : ''
  if (raw && existsSync(raw)) {
    const webm = join(outDir, name + '.webm')
    renameSync(raw, webm)
    file = name + '.webm'
    if (toMP4(webm, join(outDir, name + '.mp4'))) {
      rmSync(webm, { force: true })
      file = name + '.mp4'
    }
  }
  rmSync(tmpVideo, { recursive: true, force: true })
  return { id: sc.id, title: redact(sc.title), covers: sc.covers || [], expected: redact(sc.expected || ''), status, video: file,
    poster: name + '.png', parts, recorded_at: new Date().toISOString() }
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
  const out = { ...m, title: board.title || m.title || '', generated_by: 'test-evidence-video', updated_at: new Date().toISOString(), cases }
  if (board.suites) out.suites = board.suites
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
    console.log(`${r.status === 'passed' ? '✓' : '✗'} ${r.id} ${r.status} → ${r.video || 'no video'} (preview ${r.poster})`)
    for (const p of r.parts) if (p.status !== 'passed') console.log(`    part ${p.action}: ${p.error || p.status}${p.log ? ` (log ${p.log})` : ''}`)
  }
} finally {
  await browser.close()
  for (const d of readdirSync(outDir)) if (d.startsWith('.rec-')) rmSync(join(outDir, d), { recursive: true, force: true })
}
process.exit(results.every((r) => r.status === 'passed') ? 0 : 1)
