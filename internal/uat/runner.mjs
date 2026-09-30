// UAT screenshot runner: drives each scenario of a UAT plan in a real browser and saves a
// screenshot after every step. Invoked by the orchestrator as `node runner.mjs <job.json>`; writes
// the step results as JSON to the job's results path. Never prints form values.
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs'
import { join } from 'node:path'
import { chromium } from 'playwright-core'

const job = JSON.parse(readFileSync(process.argv[2], 'utf8'))
const STEP_TIMEOUT = job.step_timeout_ms || 15000
mkdirSync(job.out_dir, { recursive: true })

// ${UAT_*} placeholders come from the environment so secrets never live in the plan or guide.
const resolve = (v) => (v || '').replace(/\$\{(UAT_[A-Z0-9_]+)\}/g, (_, k) => process.env[k] ?? '')

function locate(page, target) {
  const t = (target || '').trim()
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
        switch (st.action) {
          case 'goto': await page.goto(st.target, { waitUntil: 'domcontentloaded' }); break
          case 'click': await locate(page, st.target).click(); break
          case 'fill': await locate(page, st.target).fill(resolve(st.value)); break
          case 'select': await locate(page, st.target).selectOption({ label: resolve(st.value) }).catch(() => locate(page, st.target).selectOption(resolve(st.value))); break
          case 'check': await locate(page, st.target).check(); break
          case 'press': await (st.target ? locate(page, st.target).press(st.value || 'Enter') : page.keyboard.press(st.value || 'Enter')); break
          case 'wait':
            if (st.target) await locate(page, st.target).waitFor({ state: 'visible' })
            else await page.waitForTimeout(Math.min(parseInt(st.value || '1000', 10) || 1000, 10000))
            break
          case 'expect_text': await page.getByText(st.value, { exact: false }).first().waitFor({ state: 'visible' }); break
        }
        await page.waitForLoadState('networkidle', { timeout: 5000 }).catch(() => {})
      } catch (err) {
        res.ok = false
        res.error = String(err && err.message ? err.message : err).split('\n')[0].slice(0, 300)
        broken = `step ${i + 1}`
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
