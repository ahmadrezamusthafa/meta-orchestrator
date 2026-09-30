// UAT screenshot runner: drives each scenario of a UAT plan in a real browser and saves an annotated
// screenshot for every step (header with scenario and step, highlighted target, callout, expected
// result, status). Invoked by the orchestrator as `node runner.mjs <job.json>`; writes
// the step results as JSON to the job's results path. Never prints form values.
import { readFileSync, writeFileSync, mkdirSync, chmodSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { chromium } from 'playwright-core'

const job = JSON.parse(readFileSync(process.argv[2], 'utf8'))
const STEP_TIMEOUT = job.step_timeout_ms || 15000
if (job.out_dir) mkdirSync(job.out_dir, { recursive: true })

// ${UAT_*} placeholders come from the environment so secrets and test-data ids never live in the
// plan or guide. A placeholder left without a value fails the step instead of opening a wrong page.
const resolve = (v) => (v || '').replace(/\$\{(UAT_[A-Z0-9_]+)\}/g, (m, k) => process.env[k] ?? m)

class StepError extends Error {
  constructor(message, reason) { super(message); this.reason = reason }
}

function resolved(v) {
  const out = resolve(v)
  const left = [...out.matchAll(/\$\{([A-Za-z0-9_]+)\}/g)].map(m => m[1])
  if (left.length) {
    const names = [...new Set(left)]
    throw new StepError(`needs test data: no value for ${names.map(n => '${' + n + '}').join(', ')} — ` +
      `fill it under Environments › Test data in the UAT Guide tab (credentials: the orchestrator's environment)`, 'placeholder')
  }
  return out
}

const pathOf = (u) => { try { const x = new URL(u); return x.origin + x.pathname } catch { return u } }

// Short pages that read like an error page: the SPA's own not-found route answers HTTP 200.
const NOT_FOUND = /\b404\b|not found|page (?:you|does not)|tidak ditemukan|halaman yang anda cari|telah dipindahkan atau telah dihapus/i

// A sign-in screen: the environment redirected to its identity provider (another host) or shows a
// password form. Scenarios that sign in themselves (fill a password) are allowed to land there.
async function onLoginPage(page, envURL) {
  let envHost = ''
  try { envHost = new URL(envURL).host } catch {}
  const host = (() => { try { return new URL(page.url()).host } catch { return '' } })()
  const password = await page.locator('input[type="password"]').first().isVisible().catch(() => false)
  return password && (host !== envHost || /login|sign[-_ ]?in|auth|account/i.test(page.url()))
}

async function checkPage(page, response) {
  const status = response ? response.status() : 0
  if (status >= 400) {
    throw new StepError(`HTTP ${status} at ${pathOf(page.url())} — check the environment URL and the path`, 'not_found')
  }
  const text = await page.evaluate(() => (document.body ? document.body.innerText : '')).catch(() => '')
  if (text.trim().length < 400 && NOT_FOUND.test(text)) {
    throw new StepError(`the app shows its "page not found" screen at ${pathOf(page.url())} — the path is not a route of this app ` +
      `(check that it includes the app's base path, e.g. /billing, and a real record id)`, 'not_found')
  }
}

// A single-page app paints after DOMContentLoaded; wait until the page has visible content so the
// screenshot is not blank.
async function settle(page) {
  await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
  await page.waitForFunction(() => document.body && document.body.innerText.trim().length > 0, null, { timeout: 5000 }).catch(() => {})
}

let current = { envURL: '', signsIn: false }

async function open(page, url) {
  const timeout = Math.max(STEP_TIMEOUT, 30000)
  // Slow staging pages: wait for the document, and on a timeout accept the page once it has started.
  const response = await page.goto(url, { waitUntil: 'domcontentloaded', timeout })
    .catch((err) => /Timeout/.test(String(err)) ? page.goto(url, { waitUntil: 'commit', timeout }) : Promise.reject(err))
  await settle(page)
  if (!current.signsIn && await onLoginPage(page, current.envURL)) {
    throw new StepError(`redirected to sign-in (${pathOf(page.url())}) — no saved session, or it expired. ` +
      'Use "Sign in" for this application in the UAT Guide tab, then regenerate', 'login_required')
  }
  await checkPage(page, response)
}

// Login mode: a visible browser window where the tester signs in once; the session (cookies and
// local storage for every site visited, SSO included) is saved as a Playwright storage state.
async function login() {
  const opts = { headless: !!job.headless, ignoreDefaultArgs: ['--enable-automation'], args: ['--disable-blink-features=AutomationControlled'] }
  // Real Chrome first: some identity providers (Google) refuse sign-in from a bundled test browser.
  const browser = await chromium.launch({ ...opts, channel: 'chrome' }).catch(() => chromium.launch(opts))
  const context = await browser.newContext({ viewport: null, ignoreHTTPSErrors: !!job.ignore_https_errors })
  const page = await context.newPage()
  const deadline = Date.now() + (job.timeout_ms || 5 * 60 * 1000)
  let envHost = ''
  try { envHost = new URL(job.url).host } catch {}
  const result = { ok: false }
  try {
    await page.goto(job.url, { waitUntil: 'domcontentloaded' }).catch(() => {})
    let signedInSince = 0
    // Done when the window is back on the application, without a password form, for 3 seconds —
    // or when the tester closes the window after signing in.
    while (Date.now() < deadline) {
      if (page.isClosed()) break
      const host = (() => { try { return new URL(page.url()).host } catch { return '' } })()
      const onApp = host === envHost && !(await onLoginPage(page, job.url))
      signedInSince = onApp ? (signedInSince || Date.now()) : 0
      if (signedInSince && Date.now() - signedInSince > 3000) break
      await new Promise((r) => setTimeout(r, 500))
    }
    const state = await context.storageState()
    const cookies = (state.cookies || []).length
    if (!cookies) throw new Error('no session was created — sign in before closing the window')
    mkdirSync(dirname(job.storage_path), { recursive: true, mode: 0o700 })
    writeFileSync(job.storage_path, JSON.stringify(state), { mode: 0o600 })
    chmodSync(job.storage_path, 0o600)
    result.ok = true
    result.cookies = cookies
    result.signed_in = !!signedInSince
  } catch (err) {
    result.error = String(err && err.message ? err.message : err).split('\n')[0].slice(0, 300)
  } finally {
    await browser.close().catch(() => {})
    writeFileSync(job.results_path, JSON.stringify(result))
  }
}

if (job.mode === 'login') {
  await login()
  process.exit(0)
}

function locate(page, target) {
  const t = resolved(target).trim()
  const [kind, ...rest] = t.split('=')
  const v = rest.join('=')
  switch (kind) {
    case 'label': return page.getByLabel(v, { exact: false }).first()
    case 'placeholder': return page.getByPlaceholder(v).first()
    case 'testid': return page.getByTestId(v).first()
    default: return page.locator(t).first() // text=, role=, css=, xpath= and plain CSS
  }
}

// Every reasonable way a tester would recognise the target, tried in each frame (microfrontends
// often render inside an iframe). The plan's own selector comes first.
function candidates(scope, target) {
  const t = resolved(target).trim()
  const [kind, ...rest] = t.split('=')
  const v = rest.join('=').replace(/^["']|["']$/g, '')
  const role = t.match(/^role=(\w+)\[name=["']?([^"'\]]+)["']?\]/)
  const out = []
  const add = (f) => { try { out.push(f()) } catch {} }
  add(() => locate(scope, t))
  const name = role ? role[2] : v
  if (role) {
    add(() => scope.getByRole(role[1], { name, exact: false }))
  }
  if (kind === 'label' || kind === 'placeholder') {
    add(() => scope.getByLabel(name, { exact: false }))
    add(() => scope.getByPlaceholder(name, { exact: false }))
    add(() => scope.getByRole('textbox', { name, exact: false }))
    add(() => scope.getByRole('combobox', { name, exact: false }))
    add(() => scope.locator(`[name="${name}" i], [aria-label*="${name}" i], [data-testid*="${name}" i]`))
  }
  if (kind === 'text' || role) {
    for (const r of ['button', 'link', 'menuitem', 'tab', 'option', 'checkbox', 'radio']) add(() => scope.getByRole(r, { name, exact: false }))
    add(() => scope.getByText(name, { exact: false }))
    add(() => scope.getByTitle(name, { exact: false }))
  }
  if (kind === 'testid') {
    add(() => scope.locator(`[data-testid="${v}"], [data-test-id="${v}"], [data-test="${v}"], [data-qa="${v}"], [data-cy="${v}"], [id="${v}"]`))
  }
  return out.map((l) => l.first())
}

// Finds the first visible candidate in any frame, waiting up to timeout for a slow screen.
async function findTarget(page, target, timeout = STEP_TIMEOUT) {
  const deadline = Date.now() + timeout
  for (;;) {
    for (const frame of page.frames()) {
      for (const loc of candidates(frame, target)) {
        if (await loc.isVisible().catch(() => false)) return loc
      }
    }
    if (Date.now() > deadline) throw new Error(`Timeout ${timeout}ms exceeded waiting for ${target}`)
    await page.waitForTimeout(300)
  }
}

// Visible text anywhere on the screen (any frame), ignoring case and extra spaces.
async function findText(page, text, timeout = STEP_TIMEOUT) {
  const want = resolved(text).replace(/\s+/g, ' ').trim()
  const deadline = Date.now() + timeout
  for (;;) {
    for (const frame of page.frames()) {
      const loc = frame.getByText(want, { exact: false }).first()
      if (await loc.isVisible().catch(() => false)) return loc
      const re = new RegExp(want.replace(/[.*+?^${}()|[\]\\]/g, '\\$&').replace(/\s+/g, '\\s+'), 'i')
      const loose = frame.getByText(re).first()
      if (await loose.isVisible().catch(() => false)) return loose
    }
    if (Date.now() > deadline) throw new Error(`Timeout ${timeout}ms exceeded waiting for text`)
    await page.waitForTimeout(300)
  }
}

async function launch() {
  const opts = { headless: true }
  try {
    return await chromium.launch(opts)
  } catch (err) {
    // No bundled browser for this playwright-core build: fall back to the installed Chrome.
    return await chromium.launch({ ...opts, channel: 'chrome' }).catch(() => { throw err })
  }
}

// ---------------------------------------------------------------------------------------------
// Annotation: each screenshot is framed for testers — a header (scenario tag, step N of M, title,
// where, status) above the untouched page capture, the target highlighted with a numbered marker and
// a callout, and a panel below with what to do, the expected result and any failure. The frame is
// rendered in its own page from the raw capture, so the application's DOM is never modified.
// ---------------------------------------------------------------------------------------------

const ACTION_VERB = {
  goto: 'Open', click: 'Click', fill: 'Type', select: 'Choose', check: 'Tick', press: 'Press', wait: 'Wait for', expect_text: 'Check',
}

const esc = (t) => String(t ?? '').replace(/[&<>"']/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]))

function frameHTML(info, png) {
  const tone = {
    pass: ['#047857', '#d1fae5', '✓ Passed'], check: ['#b45309', '#fef3c7', '⚠ Check by hand'], todo: ['#6d28d9', '#ede9fe', '➜ Do this'],
    manual: ['#1d4ed8', '#dbeafe', '✋ Manual check'], engineer: ['#0f766e', '#ccfbf1', '⚙ Engineer step'],
  }[info.status] || ['#334155', '#e2e8f0', info.status]
  const accent = info.status === 'check' ? '#d97706' : '#f59e0b'
  const b = info.box
  const W = info.width
  const H = info.height
  let marks = ''
  if (b) {
    // Below the target, else above it, else beside it — never over the controls in its own row.
    const cw = 340
    const ch = info.value ? 118 : 86
    const cx = Math.min(Math.max(b.x - 6, 12), W - cw - 12)
    let cy = b.y + b.height + 22
    let side = false
    if (cy + ch > H - 8) cy = b.y - ch - 22
    if (cy < 8) {
      side = true
      cy = Math.min(Math.max(b.y - 8, 8), H - ch - 8)
    }
    const sx = b.x + b.width + 20 + cw <= W - 12 ? b.x + b.width + 20 : Math.max(12, b.x - cw - 20)
    marks = `
      <div class="spot" style="left:${b.x - 6}px;top:${b.y - 6}px;width:${b.width + 12}px;height:${b.height + 12}px"></div>
      <div class="ring" style="left:${b.x - 6}px;top:${b.y - 6}px;width:${b.width + 12}px;height:${b.height + 12}px;border-color:${accent}"></div>
      <div class="num" style="left:${b.x - 22}px;top:${b.y - 22}px;background:${accent}">${info.step}</div>
      <div class="callout" style="left:${side ? sx : cx}px;top:${cy}px;width:${cw}px;border-color:${accent}">
        <div class="verb" style="color:${accent}">${esc(info.verb)}</div>
        <div class="what">${esc(info.caption)}</div>
        ${info.value ? `<div class="value">${esc(info.value)}</div>` : ''}
      </div>`
  }
  // A manual check on a real screen: the instruction floats in the top-right corner of the page.
  if (!b && png && info.status === 'manual' && info.caption) {
    marks = `<div class="callout" style="right:20px;top:20px;width:420px;border-color:#1d4ed8">
      <div class="verb" style="color:#1d4ed8">Manual check</div><div class="what">${esc(info.caption)}</div></div>`
  }
  return `<!doctype html><html><head><meta charset="utf-8"><style>
    * { box-sizing:border-box; margin:0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; }
    body { width:${W}px; background:#0f172a; }
    .bar { display:flex; align-items:center; gap:16px; padding:14px 22px; color:#f8fafc; }
    .tag { background:#7c3aed; color:#fff; font-weight:800; font-size:14px; padding:5px 11px; border-radius:7px; letter-spacing:.03em; white-space:nowrap; }
    .tag.sanity { background:#d97706; }
    .stepno { font-size:14px; color:#cbd5e1; white-space:nowrap; font-weight:600; }
    .head { flex:1; min-width:0; }
    .title { font-size:17px; font-weight:700; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .where { font-size:13px; color:#94a3b8; margin-top:3px; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; }
    .pill { font-size:13px; font-weight:800; padding:6px 14px; border-radius:999px; white-space:nowrap; }
    .shot { position:relative; width:${W}px; height:${H}px; overflow:hidden; background:#fff; }
    .shot img { display:block; width:${W}px; height:${H}px; }
    .spot { position:absolute; border-radius:10px; box-shadow:0 0 0 9999px rgba(15,23,42,.28); }
    .ring { position:absolute; border:3px solid; border-radius:10px; box-shadow:0 0 0 5px rgba(245,158,11,.28); }
    .num { position:absolute; width:34px; height:34px; border-radius:50%; color:#fff; font-weight:800; font-size:16px; display:flex;
           align-items:center; justify-content:center; border:3px solid #fff; box-shadow:0 2px 8px rgba(0,0,0,.35); }
    .callout { position:absolute; background:#fff; border-left:6px solid; border-radius:10px; padding:11px 15px; color:#0f172a;
               box-shadow:0 12px 32px rgba(15,23,42,.35); }
    .verb { font-size:11px; font-weight:800; text-transform:uppercase; letter-spacing:.09em; }
    .what { font-size:15px; font-weight:600; margin-top:3px; line-height:1.4; }
    .value { margin-top:7px; font:600 13px ui-monospace, SFMono-Regular, Menlo, monospace; background:#f1f5f9; padding:4px 9px;
             border-radius:6px; display:inline-block; color:#334155; }
    .panel { display:grid; grid-template-columns: 1fr 1fr; gap:12px; padding:14px 22px; background:#f8fafc; border-top:1px solid #e2e8f0; }
    .box { border-radius:10px; padding:11px 15px; font-size:14px; line-height:1.45; color:#0f172a; background:#fff; border:1px solid #e2e8f0; }
    .box h4 { font-size:11px; font-weight:800; text-transform:uppercase; letter-spacing:.08em; margin-bottom:4px; }
    .do h4 { color:#6d28d9; } .expect { background:#ecfdf5; border-color:#a7f3d0; } .expect h4 { color:#047857; }
    .error { grid-column:1 / -1; background:#fef2f2; border-color:#fecaca; color:#7f1d1d; } .error h4 { color:#b91c1c; }
    .card { width:${W}px; height:${H}px; background:linear-gradient(135deg,#f8fafc,#eef2ff); display:flex; align-items:center; justify-content:center; }
    .sheet { width:980px; background:#fff; border-radius:18px; box-shadow:0 20px 50px rgba(15,23,42,.18); padding:34px 40px; border-top:8px solid; }
    .kind { font-size:13px; font-weight:800; text-transform:uppercase; letter-spacing:.1em; }
    .big { font-size:26px; font-weight:700; color:#0f172a; margin-top:10px; line-height:1.35; }
    .req { margin-top:18px; font:700 20px ui-monospace, SFMono-Regular, Menlo, monospace; color:#0f172a; background:#f1f5f9; border-radius:10px; padding:12px 16px; }
    .req .m { color:#fff; background:#0f766e; border-radius:6px; padding:2px 10px; margin-right:12px; }
    pre { margin-top:12px; font:13px/1.5 ui-monospace, SFMono-Regular, Menlo, monospace; color:#334155; background:#f8fafc; border:1px solid #e2e8f0;
          border-radius:10px; padding:12px 16px; max-height:330px; overflow:hidden; white-space:pre-wrap; word-break:break-word; }
    .meta { margin-top:18px; display:flex; flex-wrap:wrap; gap:10px; }
    .chip { font-size:13px; color:#334155; background:#f1f5f9; border-radius:999px; padding:6px 12px; }
    .note { margin-top:18px; font-size:14px; color:#92400e; background:#fffbeb; border:1px solid #fde68a; border-radius:10px; padding:10px 14px; }
    .foot { display:flex; gap:18px; padding:7px 22px; font-size:12px; color:#94a3b8; background:#0f172a; }
    .foot .url { flex:1; min-width:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; color:#cbd5e1; font-family: ui-monospace, Menlo, monospace; }
  </style></head><body>
    <div class="bar">
      <span class="tag ${info.sanity ? 'sanity' : ''}">${esc(info.scenario)}${info.sanity ? ' · SANITY' : ''}</span>
      <span class="stepno">Step ${info.step} of ${info.total}</span>
      <div class="head"><div class="title">${esc(info.title)}</div>${info.where ? `<div class="where">📍 ${esc(info.where)}</div>` : ''}</div>
      <span class="pill" style="background:${tone[1]};color:${tone[0]}">${tone[2]}</span>
    </div>
    ${png ? `<div class="shot"><img src="data:image/png;base64,${png}">${marks}</div>` : cardHTML(info)}
    <div class="panel">
      <div class="box do"><h4>Step ${info.step} · ${esc(info.verb)}</h4>${esc(info.caption)}${info.value ? ` <span class="value">${esc(info.value)}</span>` : ''}</div>
      <div class="box expect"><h4>Expected result</h4>${esc(info.expected || {
        manual: 'Confirm it by hand, then tick Pass or Fail.', engineer: 'The response matches the expected result of this case.',
      }[info.status] || (png ? 'The screen matches this screenshot.' : 'The screen shows the result described in this step.'))}</div>
      ${info.error ? `<div class="box error"><h4>The automated walkthrough could not confirm this step — check it by hand</h4>${esc(info.error)}</div>` : ''}
    </div>
    <div class="foot"><span>${esc(info.label)}</span><span>${esc(info.app)}</span><span class="url">${esc(info.url)}</span><span>${esc(info.at)}</span></div>
  </body></html>`
}

// The screen area of a step with no page to show: an API request, a manual check, or a scenario that
// is not replayed (no environment URL, or written from the ATDD sheet). Same frame, same style.
function cardHTML(info) {
  const color = { engineer: '#0f766e', manual: '#1d4ed8' }[info.status] || '#6d28d9'
  const kind = { engineer: 'Engineer step · API request', manual: 'Manual check' }[info.status] || `${info.verb} · on screen`
  let body = `<div class="big">${esc(info.caption)}</div>`
  if (info.action === 'api') {
    const [method, ...path] = String(info.target || '').trim().split(/\s+/)
    let payload = info.body || ''
    try { if (payload) payload = JSON.stringify(JSON.parse(payload), null, 2) } catch {}
    body = `<div class="big">${esc(info.caption)}</div>
      ${info.target ? `<div class="req"><span class="m">${esc(method)}</span>${esc(path.join(' '))}</div>` : ''}
      ${payload ? `<pre>${esc(payload.length > 2400 ? payload.slice(0, 2400) + '\n…' : payload)}</pre>` : ''}`
  } else if (info.target && info.action !== 'manual') {
    body += `<div class="meta"><span class="chip">🎯 ${esc(info.targetText)}</span>${info.value ? `<span class="chip">⌨ ${esc(info.value)}</span>` : ''}</div>`
  }
  const meta = [info.where && `📍 ${esc(info.where)}`, info.executable && `👤 ${esc(info.executable)}`].filter(Boolean)
  return `<div class="card"><div class="sheet" style="border-color:${color}">
      <div class="kind" style="color:${color}">${esc(kind)}</div>
      ${body}
      ${meta.length ? `<div class="meta">${meta.map((m) => `<span class="chip">${m}</span>`).join('')}</div>` : ''}
      ${info.note ? `<div class="note">${esc(info.note)}</div>` : ''}
    </div></div>`
}

let framer = null // one page that renders every frame

async function renderFrame(info, png, path) {
  if (!framer) {
    const ctx = await browser.newContext({ viewport: { width: info.width, height: 600 }, deviceScaleFactor: job.scale || 2 })
    framer = await ctx.newPage()
  }
  await framer.setViewportSize({ width: info.width, height: 600 })
  await framer.setContent(frameHTML(info, png), { waitUntil: 'load' })
  await framer.screenshot({ path, fullPage: true })
}

// The element the step acts on (or the text it checks for), scrolled into view below the top bar.
async function targetBox(page, st) {
  let loc = null
  try {
    if (st.action === 'expect_text') loc = await findText(page, st.value, 1500)
    else if (st.target && !['goto', 'manual', 'api'].includes(st.action)) loc = await findTarget(page, st.target, 1500)
  } catch { return null }
  if (!loc) return null
  try {
    await loc.evaluate((el) => el.scrollIntoView({ block: 'center', inline: 'nearest' })).catch(() => {})
    const box = await loc.boundingBox()
    // A box inside an iframe is already in page coordinates; ignore anything off screen.
    return box && box.width > 0 && box.height > 0 ? box : null
  } catch { return null }
}

// Secrets are never drawn: a ${UAT_…PASSWORD…}-style value shows as dots.
function shownValue(st) {
  if (!['fill', 'select', 'press'].includes(st.action) || !st.value) return ''
  if (/\$\{UAT_[A-Z0-9_]*(PASS|PWD|TOKEN|SECRET|KEY|OTP|PIN)[A-Z0-9_]*\}/.test(st.value) || /password/i.test(st.target || '')) return '••••••••'
  return st.action === 'press' ? `⌨ ${st.value}` : resolve(st.value)
}

// What the target is, in the words a tester sees on screen.
function describeTarget(target) {
  const t = resolve(target || '').trim()
  const [kind, ...rest] = t.split('=')
  const v = rest.join('=').replace(/^["']|["']$/g, '')
  const role = t.match(/^role=(\w+)\[name=["']?([^"'\]]+)/)
  if (role) return `the ${role[1]} “${role[2]}”`
  switch (kind) {
    case 'label': return `the field “${v}”`
    case 'placeholder': return `the field showing “${v}”`
    case 'text': return `“${v}”`
    case 'testid': return `the element marked ${v}`
    default: return `the element ${t}`
  }
}

// Turns a Playwright failure into a sentence a business tester understands; the automation detail
// stays in brackets for the engineer.
function plainError(message, st) {
  const m = String(message || '')
  if (/^not run|^needs test data|^redirected to sign-in|^HTTP \d|^the app shows/.test(m)) return m
  const secs = (m.match(/Timeout (\d+)ms/) || [])[1]
  const within = secs ? ` within ${Math.round(secs / 1000)} seconds` : ''
  let plain = ''
  if (st.action === 'expect_text' && /Timeout/.test(m)) plain = `The text “${resolve(st.value)}” did not appear on the screen${within}.`
  else if (/Timeout/.test(m) && st.target) plain = `Could not find ${describeTarget(st.target)} on the screen${within}.`
  else if (/not visible|not enabled|disabled|intercepts pointer/.test(m)) plain = `${describeTarget(st.target)} is on the screen but cannot be used yet (hidden, disabled or covered).`
  else if (/did not find some options|No option/.test(m)) plain = `The option “${resolve(st.value)}” is not in ${describeTarget(st.target)}.`
  else if (/net::|ERR_|NS_ERROR/.test(m)) plain = 'The page could not be loaded — check the environment URL and your connection.'
  return plain ? `${plain} [${m}]` : m
}

function stepInfo(sc, i, st, status, error) {
  return {
    scenario: sc.id, sanity: !!sc.sanity, title: sc.title || '', step: i + 1, total: sc.steps.length,
    where: st.where || '', verb: st.action === 'api' ? 'API request' : st.action === 'manual' ? 'Manual check' : ACTION_VERB[st.action] || st.action,
    caption: (sc.captions || [])[i] || st.description || '', action: st.action, target: maskSecrets(st.target || ''), body: maskSecrets(st.value || ''),
    targetText: st.target ? describeTarget(st.target) : '', executable: sc.executable_by || '', note: sc.card_note || '',
    value: shownValue(st), expected: st.expected || '', error: (error || '').replace(/\s*\[[^\]]*\]$/, ''), status, box: null,
    width: 1440, height: 900, label: job.label || 'UAT', app: sc.app_name || '', url: '',
    at: new Date().toLocaleString('en-GB', { dateStyle: 'medium', timeStyle: 'short' }),
  }
}

const shotName = (sc, i) => `${sc.id}-${String(i + 1).padStart(2, '0')}.png`

async function capture(page, sc, i, st, status, error) {
  const file = shotName(sc, i)
  const vp = page.viewportSize() || { width: 1440, height: 900 }
  const info = { ...stepInfo(sc, i, st, status, error), width: vp.width, height: vp.height, url: pathOf(page.url()) }
  info.box = await targetBox(page, st).catch(() => null)
  const png = (await page.screenshot({ fullPage: false })).toString('base64')
  const path = join(job.out_dir, file)
  if (job.annotate === false) writeFileSync(path, Buffer.from(png, 'base64'))
  else await renderFrame(info, png, path)
  return file
}

// A step card: the same frame, with the step drawn where the screen would be.
async function captureCard(sc, i, st, status) {
  const file = shotName(sc, i)
  const info = stepInfo(sc, i, st, status, '')
  await renderFrame(info, '', join(job.out_dir, file))
  return file
}

// ${UAT_…} credentials never appear in an image; test-data placeholders stay readable.
function maskSecrets(v) {
  return String(v || '').replace(/\$\{(UAT_[A-Z0-9_]*(?:PASS|PWD|TOKEN|SECRET|KEY|OTP|PIN|SESSION|COOKIE)[A-Z0-9_]*)\}/g, '••••••••')
}

// A click changes the screen, so it is captured just before, with the element highlighted: that is
// what the tester needs to find. Everything else is captured after, showing its result.
const CAPTURE_BEFORE = new Set(['click'])

async function act(page, st) {
  switch (st.action) {
    case 'goto': await open(page, resolved(st.target)); break
    case 'click': {
      const loc = await findTarget(page, st.target)
      await loc.click({ timeout: 5000 }).catch(async (err) => {
        // Covered by a sticky header or an animation: click the element itself.
        if (!/intercepts pointer|not stable|outside of the viewport/.test(String(err))) throw err
        await loc.dispatchEvent('click')
      })
      break
    }
    case 'fill': {
      const loc = await findTarget(page, st.target)
      await loc.fill(resolved(st.value), { timeout: 5000 }).catch(async () => {
        // Not a plain input (rich editor, masked input): type into it like a person would.
        await loc.click({ timeout: 5000 })
        await page.keyboard.press(process.platform === 'darwin' ? 'Meta+A' : 'Control+A')
        await page.keyboard.type(resolved(st.value))
      })
      break
    }
    case 'select': {
      const loc = await findTarget(page, st.target)
      const v = resolved(st.value)
      await loc.selectOption({ label: v }, { timeout: 5000 }).catch(() => loc.selectOption(v, { timeout: 5000 })).catch(async () => {
        // A custom dropdown: open it and pick the option by its text.
        await loc.click({ timeout: 5000 })
        await (await findTarget(page, `text=${v}`, 8000)).click({ timeout: 5000 })
      })
      break
    }
    case 'check': {
      const loc = await findTarget(page, st.target)
      await loc.check({ timeout: 5000 }).catch(() => loc.click({ timeout: 5000 }))
      break
    }
    case 'press':
      if (st.target) await (await findTarget(page, st.target)).press(st.value || 'Enter')
      else await page.keyboard.press(st.value || 'Enter')
      break
    case 'wait':
      if (st.target) await findTarget(page, st.target, Math.max(STEP_TIMEOUT, 20000))
      else await page.waitForTimeout(Math.min(parseInt(st.value || '1000', 10) || 1000, 10000))
      break
    case 'expect_text': await findText(page, st.value, Math.max(STEP_TIMEOUT, 20000)); break
  }
  if (st.action !== 'goto') await settle(page)
}

// One retry after the screen settles: staging pages are often just slow.
async function actWithRetry(page, st) {
  try {
    await act(page, st)
  } catch (err) {
    if (err instanceof StepError || !/Timeout|detached|not attached|navigation/i.test(String(err))) throw err
    await settle(page)
    await act(page, st)
  }
}

const results = []
const browser = await launch()
try {
  for (const sc of job.scenarios) {
    // One context per scenario: each app has its own environment and signed-in session.
    const context = await browser.newContext({
      baseURL: sc.base_url || undefined,
      viewport: { width: 1440, height: 900 },
      deviceScaleFactor: job.scale || 2, // crisp text in the guide
      storageState: sc.storage_state || undefined,
      ignoreHTTPSErrors: !!job.ignore_https_errors,
    })
    const page = await context.newPage()
    page.setDefaultTimeout(STEP_TIMEOUT)
    current = { envURL: sc.base_url, signsIn: sc.steps.some((st) => st.action === 'fill' && /password|UAT_PASSWORD/i.test(`${st.target} ${st.value}`)) }
    for (let i = 0; i < sc.steps.length; i++) {
      const st = sc.steps[i]
      const res = { scenario: sc.id, step: i + 1, ok: true }
      if (!sc.replay || st.action === 'api') {
        // No screen for this step: an API request, or a scenario that is not replayed.
        res.skipped = true
        res.screenshot = await captureCard(sc, i, st, st.action === 'api' ? 'engineer' : st.action === 'manual' ? 'manual' : 'todo').catch(() => undefined)
        res.phase = 'card'
        results.push(res)
        continue
      }
      if (st.action === 'manual') {
        // Show where the tester is when they make the check; a card when nothing is open yet.
        res.skipped = true
        res.screenshot = page.url() === 'about:blank'
          ? await captureCard(sc, i, st, 'manual').catch(() => undefined)
          : await capture(page, sc, i, st, 'manual', '').catch(() => undefined)
        res.phase = page.url() === 'about:blank' ? 'card' : 'after'
        results.push(res)
        continue
      }
      let before = false
      try {
        // Nothing is open yet (the scenario starts with a click or a check): start on the app's
        // environment URL instead of acting on a blank page.
        if (st.action !== 'goto' && page.url() === 'about:blank') await open(page, sc.base_url)
        if (CAPTURE_BEFORE.has(st.action)) {
          await findTarget(page, st.target)
          res.screenshot = await capture(page, sc, i, st, 'todo', '')
          res.url = page.url().split('?')[0]
          res.phase = 'before'
          before = true
        }
        await actWithRetry(page, st)
      } catch (err) {
        res.ok = false
        res.error = plainError(String(err && err.message ? err.message : err).split('\n')[0].slice(0, 300), st)
        if (err instanceof StepError) res.reason = err.reason
      }
      // Every step gets an image: the screen after it, or — for a click that could not run — the
      // screen where the tester has to find the control.
      if (!before || !res.ok) {
        try {
          res.screenshot = page.url() === 'about:blank'
            ? await captureCard(sc, i, st, res.ok ? 'todo' : 'check')
            : await capture(page, sc, i, st, res.ok ? 'pass' : 'check', res.error)
          res.url = page.url().split('?')[0]
          res.phase = page.url() === 'about:blank' ? 'card' : 'after'
        } catch (err) {
          res.error = res.error || `screenshot failed: ${String(err).slice(0, 200)}`
        }
      }
      results.push(res)
    }
    await context.close()
  }
} finally {
  await browser.close()
  writeFileSync(job.results_path, JSON.stringify(results, null, 2))
}
