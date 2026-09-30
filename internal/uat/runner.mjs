// UAT screenshot runner: drives each scenario of a UAT plan in a real browser and saves a
// screenshot after every step. Invoked by the orchestrator as `node runner.mjs <job.json>`; writes
// the step results as JSON to the job's results path. Never prints form values.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { chromium } from 'playwright-core'

const job = JSON.parse(readFileSync(process.argv[2], 'utf8'))
const STEP_TIMEOUT = job.step_timeout_ms || 15000
mkdirSync(job.out_dir, { recursive: true })

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

async function open(page, url) {
  const response = await page.goto(url, { waitUntil: 'domcontentloaded' })
  await settle(page)
  await checkPage(page, response)
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

const results = []
const browser = await launch()
try {
  for (const sc of job.scenarios) {
    // One context per scenario: each app has its own environment and signed-in session.
    const context = await browser.newContext({
      baseURL: sc.base_url,
      viewport: { width: 1440, height: 900 },
      storageState: sc.storage_state || undefined,
      ignoreHTTPSErrors: !!job.ignore_https_errors,
    })
    const page = await context.newPage()
    page.setDefaultTimeout(STEP_TIMEOUT)
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
      try {
        // Nothing is open yet (the scenario starts with a click or a check): start on the app's
        // environment URL instead of acting on a blank page.
        if (st.action !== 'goto' && page.url() === 'about:blank') await open(page, sc.base_url)
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
      } catch (err) {
        res.ok = false
        res.error = String(err && err.message ? err.message : err).split('\n')[0].slice(0, 300)
        if (err instanceof StepError) res.reason = err.reason
        broken = `step ${i + 1}`
      }
      // A blank page proves nothing: capture only once something is open.
      if (page.url() === 'about:blank') {
        results.push(res)
        continue
      }
      try {
        const file = `${sc.id}-${String(i + 1).padStart(2, '0')}.png`
        await page.screenshot({ path: join(job.out_dir, file), fullPage: false })
        res.screenshot = file
        res.url = page.url().split('?')[0]
      } catch (err) {
        res.error = res.error || `screenshot failed: ${String(err).slice(0, 200)}`
      }
      results.push(res)
    }
    await context.close()
  }
} finally {
  await browser.close()
  writeFileSync(job.results_path, JSON.stringify(results, null, 2))
}
