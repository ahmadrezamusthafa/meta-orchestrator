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
  const response = await page.goto(url, { waitUntil: 'domcontentloaded' })
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
  const tone = { pass: ['#047857', '#d1fae5', '✓ Passed'], fail: ['#b91c1c', '#fee2e2', '✕ Failed'], todo: ['#6d28d9', '#ede9fe', '➜ Do this'] }[info.status]
  const accent = info.status === 'fail' ? '#dc2626' : '#f59e0b'
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
    .foot { display:flex; gap:18px; padding:7px 22px; font-size:12px; color:#94a3b8; background:#0f172a; }
    .foot .url { flex:1; min-width:0; white-space:nowrap; overflow:hidden; text-overflow:ellipsis; color:#cbd5e1; font-family: ui-monospace, Menlo, monospace; }
  </style></head><body>
    <div class="bar">
      <span class="tag ${info.sanity ? 'sanity' : ''}">${esc(info.scenario)}${info.sanity ? ' · SANITY' : ''}</span>
      <span class="stepno">Step ${info.step} of ${info.total}</span>
      <div class="head"><div class="title">${esc(info.title)}</div>${info.where ? `<div class="where">📍 ${esc(info.where)}</div>` : ''}</div>
      <span class="pill" style="background:${tone[1]};color:${tone[0]}">${tone[2]}</span>
    </div>
    <div class="shot"><img src="data:image/png;base64,${png}">${marks}</div>
    <div class="panel">
      <div class="box do"><h4>Step ${info.step} · ${esc(info.verb)}</h4>${esc(info.caption)}${info.value ? ` <span class="value">${esc(info.value)}</span>` : ''}</div>
      <div class="box expect"><h4>Expected result</h4>${esc(info.expected || 'The screen matches this screenshot.')}</div>
      ${info.error ? `<div class="box error"><h4>Automated check failed — verify by hand</h4>${esc(info.error)}</div>` : ''}
    </div>
    <div class="foot"><span>${esc(info.label)}</span><span>${esc(info.app)}</span><span class="url">${esc(info.url)}</span><span>${esc(info.at)}</span></div>
  </body></html>`
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
    if (st.action === 'expect_text') loc = page.getByText(resolved(st.value), { exact: false }).first()
    else if (st.target && st.action !== 'goto' && !(st.action === 'wait' && !st.target)) loc = locate(page, st.target)
  } catch { return null }
  if (!loc) return null
  try {
    if (!(await loc.isVisible({ timeout: 1000 }))) return null
    await loc.evaluate((el) => el.scrollIntoView({ block: 'center', inline: 'nearest' })).catch(() => {})
    return await loc.boundingBox()
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

async function capture(page, sc, i, st, status, error) {
  const file = `${sc.id}-${String(i + 1).padStart(2, '0')}.png`
  const box = error?.startsWith('not run') ? null : await targetBox(page, st).catch(() => null)
  const vp = page.viewportSize() || { width: 1440, height: 900 }
  const png = (await page.screenshot({ fullPage: false })).toString('base64')
  const info = {
    scenario: sc.id, sanity: !!sc.sanity, title: sc.title || '', step: i + 1, total: sc.steps.length,
    where: st.where || '', verb: ACTION_VERB[st.action] || st.action, caption: (sc.captions || [])[i] || st.description || '',
    value: shownValue(st), expected: st.expected || '', error: (error || '').replace(/\s*\[[^\]]*\]$/, ''), status, box, width: vp.width, height: vp.height,
    label: job.label || 'UAT', app: sc.app_name || '', url: pathOf(page.url()),
    at: new Date().toLocaleString('en-GB', { dateStyle: 'medium', timeStyle: 'short' }),
  }
  const path = join(job.out_dir, file)
  if (job.annotate === false) writeFileSync(path, Buffer.from(png, 'base64'))
  else await renderFrame(info, png, path)
  return file
}

// A click changes the screen, so it is captured just before, with the element highlighted: that is
// what the tester needs to find. Everything else is captured after, showing its result.
const CAPTURE_BEFORE = new Set(['click'])

async function act(page, st) {
  switch (st.action) {
    case 'goto': await open(page, resolved(st.target)); break
    case 'click': await locate(page, st.target).click(); break
    case 'fill': await locate(page, st.target).fill(resolved(st.value)); break
    case 'select': await locate(page, st.target).selectOption({ label: resolved(st.value) }).catch(() => locate(page, st.target).selectOption(resolved(st.value))); break
    case 'check': await locate(page, st.target).check(); break
    case 'press': await (st.target ? locate(page, st.target).press(st.value || 'Enter') : page.keyboard.press(st.value || 'Enter')); break
    case 'wait':
      if (st.target) await locate(page, st.target).waitFor({ state: 'visible' })
      else await page.waitForTimeout(Math.min(parseInt(st.value || '1000', 10) || 1000, 10000))
      break
    case 'expect_text': await page.getByText(resolved(st.value), { exact: false }).first().waitFor({ state: 'visible' }); break
  }
  if (st.action !== 'goto') await settle(page)
}

const results = []
const browser = await launch()
try {
  for (const sc of job.scenarios) {
    // One context per scenario: each app has its own environment and signed-in session.
    const context = await browser.newContext({
      baseURL: sc.base_url,
      viewport: { width: 1440, height: 900 },
      deviceScaleFactor: job.scale || 2, // crisp text in the guide
      storageState: sc.storage_state || undefined,
      ignoreHTTPSErrors: !!job.ignore_https_errors,
    })
    const page = await context.newPage()
    page.setDefaultTimeout(STEP_TIMEOUT)
    current = { envURL: sc.base_url, signsIn: sc.steps.some((st) => st.action === 'fill' && /password|UAT_PASSWORD/i.test(`${st.target} ${st.value}`)) }
    let broken = ''
    for (let i = 0; i < sc.steps.length; i++) {
      const st = sc.steps[i]
      const res = { scenario: sc.id, step: i + 1, ok: true }
      if (st.action === 'manual' || st.action === 'api') {
        res.skipped = true
        results.push(res)
        continue
      }
      if (broken) {
        res.ok = false
        res.error = `not run: an earlier step failed (${broken})`
        results.push(res)
        continue
      }
      let before = false
      try {
        // Nothing is open yet (the scenario starts with a click or a check): start on the app's
        // environment URL instead of acting on a blank page.
        if (st.action !== 'goto' && page.url() === 'about:blank') await open(page, sc.base_url)
        if (CAPTURE_BEFORE.has(st.action)) {
          await locate(page, st.target).waitFor({ state: 'visible' })
          res.screenshot = await capture(page, sc, i, st, 'todo', '')
          res.url = page.url().split('?')[0]
          res.phase = 'before'
          before = true
        }
        await act(page, st)
      } catch (err) {
        res.ok = false
        res.error = plainError(String(err && err.message ? err.message : err).split('\n')[0].slice(0, 300), st)
        if (err instanceof StepError) res.reason = err.reason
        broken = `step ${i + 1}`
      }
      // A blank page proves nothing: capture only once something is open.
      if (page.url() === 'about:blank') {
        results.push(res)
        continue
      }
      // After-capture for every step, and for a click that failed (its before-shot would read "Do this").
      if (!before || !res.ok) {
        try {
          res.screenshot = await capture(page, sc, i, st, res.ok ? 'pass' : 'fail', res.error)
          res.url = page.url().split('?')[0]
          res.phase = 'after'
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
