import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'

const screenshotDir = resolve(import.meta.dirname, '../../docs/screenshots')
const stageKeys = ['constitution', 'specify', 'clarify', 'plan', 'checklist', 'tasks', 'analyze', 'implement', 'tests', 'converge', 'final_review', 'report']
const run = {
  id: 'browser-run-001',
  project: '/workspace/demo',
  prompt: 'Verify the browser workflow',
  state: 'COMPLETED',
  execution: 'SIMULATED',
  created_at: '2026-10-23T09:00:00Z',
}
const stages = stageKeys.map((key, index) => ({
  id: `stage-${index + 1}`,
  key,
  state: index === 9 ? 'BLOCKED' : 'SUCCEEDED',
  provider: 'fake',
  profile: 'browser-fixture',
  model: 'deterministic',
  effort: 'none',
  attempt: 1,
}))
const report = {
  run,
  acceptance: { verdict: 'BLOCKED', execution: 'SIMULATED', required_gates: [], simulation_gates: [] },
  stages,
  execution_attempts: [{ id: 'attempt-1', stage: 'implement', phase: 'implement', iteration: 1, provider: 'fake', profile: 'browser-fixture', model: 'deterministic', effort: 'none', started_at: run.created_at, duration_ms: 230, result: 'SUCCEEDED', exit_code: 0 }],
  quality_executions: [{ id: 'quality-1', phase: 'tests', iteration: 1, started_at: run.created_at, finished_at: run.created_at, verdict: 'PASS', results: [{ key: 'unit', verdict: 'PASS', command: ['go', 'test', './...'], duration_ns: 1_000_000, summary: 'Fixture-backed checks passed', exit_code: 0 }] }],
  gates: [
    { key: 'analyze_clean', verdict: 'PASS', evidence: 'No findings' },
    { key: 'required_tests', verdict: 'PASS', evidence: 'Fixture-backed checks passed' },
    { key: 'converged', verdict: 'BLOCKED', evidence: 'Fixture requests a blocked convergence result' },
  ],
  routing_decisions: [{ stage: 'implement', provider: 'fake', profile: 'browser-fixture', model: 'deterministic', effort: 'none', policy: 'simulated', explanation: 'Browser acceptance fixture' }],
  artifacts: [],
  approvals: [],
}
const readyPreview = {
  dry_run: true,
  execution: 'NOT_RUN',
  mode: 'hybrid',
  decisions: stageKeys.map((stage) => ({ stage, route: { provider: 'fake', profile: 'browser-fixture', model: 'deterministic', effort: 'none' }, explanation: 'Offline browser fixture' })),
}
const profile = {
  id: 'profile-1', name: 'browser-fixture', provider: 'codex', executable: 'codex', allowed_scopes: ['/workspace'],
  capability: { provider: 'codex', executable_found: true, version: '1.2.3', authenticated: 'AUTHENTICATED', read_only_execution: true, workspace_write: true },
}

type FixtureOptions = { runs?: unknown[]; profiles?: unknown[]; error?: string; versionError?: boolean; report?: unknown }

async function installAPI(page: Page, options: FixtureOptions = {}) {
  await page.route('**/api/v1/**', async (route: Route) => {
    const url = new URL(route.request().url())
    const path = url.pathname
    if (path === '/api/v1/version') {
      if (options.versionError) return route.abort()
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ version: '0.1.0-dev', csrf_token: 'browser-test-token' }) })
    }
    if (path === '/api/v1/profiles') {
      if (options.error === 'profiles') return route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"profile store unavailable"}' })
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(options.profiles ?? []) })
    }
    if (path === '/api/v1/runs' && route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { dry_run?: boolean; project?: string }
      if (body.dry_run) {
        if (body.project?.includes('missing')) return route.fulfill({ status: 400, contentType: 'application/json', body: JSON.stringify({ error: { code: 'ROUTE_PREVIEW_BLOCKED', message: 'PROJECT_PATH_UNAVAILABLE' }, execution: 'NOT_RUN' }) })
        return route.fulfill({ contentType: 'application/json', body: JSON.stringify(readyPreview) })
      }
      return route.fulfill({ status: 409, contentType: 'application/json', body: '{"error":"unexpected run creation in browser acceptance"}' })
    }
    if (path === '/api/v1/runs') {
      if (options.error === 'runs') return route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"run store unavailable"}' })
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify(options.runs ?? [run]) })
    }
    if (path === `/api/v1/runs/${run.id}`) return route.fulfill({ contentType: 'application/json', body: JSON.stringify(options.report ?? report) })
    if (path === `/api/v1/runs/${run.id}/events`) {
      const event = { run_id: run.id, seq: 1, type: 'provider.event', stage: 'implement', payload: { summary: 'fixture activity' } }
      return route.fulfill({ status: 200, contentType: 'text/event-stream; charset=utf-8', body: `event: provider.event\ndata: ${JSON.stringify(event)}\n\n` })
    }
    return route.fulfill({ status: 404, contentType: 'application/json', body: '{"error":"not found"}' })
  })
}

async function open(page: Page, options?: FixtureOptions) {
  await installAPI(page, options)
  await page.goto('/')
  await expect(page.getByRole('button', { name: /Workflow control/ })).toBeVisible()
  if (options?.versionError) await expect(page.getByRole('status', { name: 'disconnected state' }).first()).toBeVisible()
  else await expect(page.locator('.status-line')).toContainText('durable runs')
}

async function scan(page: Page, name: string) {
  const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa']).analyze()
  const serious = result.violations.filter((violation) => ['critical', 'serious'].includes(violation.impact ?? ''))
  expect(serious, `${name} accessibility violations`).toEqual([])
}

async function saveScreenshot(page: Page, name: string) {
  await mkdir(screenshotDir, { recursive: true })
  await page.screenshot({ path: resolve(screenshotDir, `${name}.png`), fullPage: true })
}

test('run history and detail render with accessible status labels', async ({ page }) => {
  await open(page)
  await page.getByRole('button', { name: /Run history/ }).click()
  await expect(page.getByRole('heading', { name: 'All runs' })).toBeVisible()
  await expect(page.getByRole('button', { name: /SIMULATED COMPLETED/ })).toBeVisible()
  await saveScreenshot(page, 'run-history-desktop')
  await scan(page, 'run-list')

  await page.getByRole('button', { name: /Workflow control/ }).click()
  await page.getByRole('button', { name: /SIMULATED COMPLETED/ }).click()
  await expect(page.getByRole('heading', { name: /Run browser-/ })).toBeVisible()
  await expect(page.locator('.detail-panel > .stage-list')).toContainText('attempt 1')
  await expect(page.locator('.detail-panel')).toContainText('Fixture-backed checks passed')
  await expect(page.locator('.detail-panel')).toContainText('BLOCKED')
  await saveScreenshot(page, 'run-detail-desktop')
  await scan(page, 'run-detail')
})

test('empty, failed, and disconnected dashboard surfaces are labeled and accessible', async ({ page }) => {
  await open(page, { runs: [] })
  await expect(page.getByText('Select a run to inspect its stages, routes, and evidence.')).toBeVisible()
  await saveScreenshot(page, 'workflow-empty-desktop')
  await scan(page, 'empty-workflow-detail')
  await page.getByRole('button', { name: /Run history/ }).click()
  await expect(page.getByText('No runs recorded yet')).toBeVisible()
  await scan(page, 'empty-run-list')

  await page.getByRole('button', { name: /Route preview/ }).click()
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await page.getByLabel('Project path').fill('./missing-project')
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await expect(page.getByRole('alert', { name: 'error state' })).toContainText('PROJECT_PATH_UNAVAILABLE')
  await saveScreenshot(page, 'route-preview-blocked-desktop')
  await scan(page, 'route-preview-error')

  const disconnectedPage = await page.context().newPage()
  await open(disconnectedPage, { versionError: true })
  await expect(disconnectedPage.getByRole('status', { name: 'disconnected state' }).first()).toBeVisible()
  await saveScreenshot(disconnectedPage, 'disconnected-desktop')
  await scan(disconnectedPage, 'disconnected')
  await disconnectedPage.close()

  const profileErrorPage = await page.context().newPage()
  await open(profileErrorPage, { error: 'profiles' })
  await profileErrorPage.getByRole('button', { name: /Profiles & models/ }).click()
  await expect(profileErrorPage.getByRole('alert')).toContainText('profile store unavailable')
  await scan(profileErrorPage, 'profile-error')
  await profileErrorPage.close()
})

test('ready route preview, profile empty states, and profile readiness are accessible', async ({ page }) => {
  await open(page)
  await page.getByRole('button', { name: /Route preview/ }).click()
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toContainText('NOT RUN')
  await expect(page.locator('.route-preview-results .route-list > div')).toHaveCount(12)
  await saveScreenshot(page, 'route-preview-desktop')
  await scan(page, 'route-preview')

  await page.getByRole('button', { name: /Profiles & models/ }).click()
  await expect(page.getByText('No profiles registered')).toBeVisible()
  await saveScreenshot(page, 'profiles-empty-desktop')
  await scan(page, 'profiles-empty')
  await page.getByRole('button', { name: /Profile readiness/ }).click()
  await expect(page.getByText('No profiles to assess')).toBeVisible()
  await saveScreenshot(page, 'profile-readiness-empty-desktop')
  await scan(page, 'profile-readiness')
})

test('two viewers agree, keyboard focus is visible, reduced motion is honored, and 200% zoom reflows', async ({ browser }) => {
  const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } })
  const first = await context.newPage()
  const second = await context.newPage()
  await open(first)
  await open(second)
  for (const page of [first, second]) {
    await page.getByRole('button', { name: /SIMULATED COMPLETED/ }).click()
    await expect(page.getByRole('heading', { name: /Run browser-/ })).toBeVisible()
    await expect(page.locator('.event-list')).toContainText('fixture')
  }
  expect(await first.locator('.detail-panel > .stage-list').innerText()).toBe(await second.locator('.detail-panel > .stage-list').innerText())

  await first.reload()
  await first.keyboard.press('Tab')
  const focus = await first.evaluate(() => ({
    label: (document.activeElement as HTMLElement | null)?.getAttribute('aria-label') ?? (document.activeElement as HTMLElement | null)?.getAttribute('placeholder') ?? (document.activeElement as HTMLElement | null)?.tagName,
    outline: getComputedStyle(document.activeElement as Element).outlineStyle,
  }))
  expect(focus.label).toBeTruthy()
  expect(focus.outline).not.toBe('none')

  await context.close()
  const reduced = await browser.newPage({ reducedMotion: 'reduce' })
  await open(reduced)
  const motion = await reduced.locator('.nav-item').first().evaluate((element) => getComputedStyle(element).transitionDuration)
  expect(motion.split(',').every((duration) => Number.parseFloat(duration) === 0)).toBe(true)
  await reduced.setViewportSize({ width: 390, height: 844 })
  await saveScreenshot(reduced, 'workflow-phone')
  const phoneOverflow = await reduced.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
  expect(phoneOverflow).toBe(false)
  await reduced.setViewportSize({ width: 1440, height: 1000 })
  await reduced.evaluate(() => { document.documentElement.style.zoom = '200%' })
  const zoomOverflow = await reduced.evaluate(() => document.documentElement.scrollWidth > document.documentElement.clientWidth)
  expect(zoomOverflow).toBe(false)
  await saveScreenshot(reduced, 'workflow-200-percent-page-scale')
})

test('profile readiness data is readable and retains non-color status text', async ({ page }) => {
  await open(page, { profiles: [profile] })
  await page.getByRole('button', { name: /Profile readiness/ }).click()
  await expect(page.getByText('READY', { exact: true })).toBeVisible()
  await scan(page, 'profile-readiness-ready')
})

test('all 26 required dashboard page and state pairs pass Axe', async ({ page, context }) => {
  test.setTimeout(300_000)
  const options: FixtureOptions = { runs: [] }
  await open(page, options)
  await page.getByRole('button', { name: /Run history/ }).click()
  await expect(page.getByText('No runs recorded yet')).toBeVisible()
  await scan(page, 'run-list-empty')
  const listStates = ['RUNNING', 'PAUSED', 'BLOCKED', 'COMPLETED', 'CANCELLED', 'SIMULATED'] as const
  for (const state of listStates) {
    const runState = state === 'SIMULATED' ? 'COMPLETED' : state
    options.runs = [{ ...run, state: runState, execution: state === 'SIMULATED' ? 'SIMULATED' : 'LIVE' }]
    await page.reload()
    await expect(page.locator('.status-line')).toContainText('durable runs')
    await page.getByRole('button', { name: /Run history/ }).click()
    await expect(page.getByRole('button', { name: new RegExp(`Verify the browser workflow.*${runState}`) })).toBeVisible()
    await scan(page, `run-list-${state.toLowerCase()}`)
  }
  const disconnectedList = await context.newPage()
  await open(disconnectedList, { versionError: true })
  await disconnectedList.getByRole('button', { name: /Run history/ }).click()
  await scan(disconnectedList, 'run-list-disconnected')
  await disconnectedList.close()

  const detailStates = ['RUNNING', 'PAUSED', 'BLOCKED', 'COMPLETED', 'CANCELLED', 'FAILED']
  for (const state of detailStates) {
    const detailRun = { ...run, state, execution: 'LIVE' }
    options.runs = [detailRun]
    options.report = { ...report, run: detailRun, approvals: state === 'PAUSED' ? [{ id: 'approval-1', action: 'Write project files', status: 'PENDING', decision: '' }] : report.approvals }
    await page.reload()
    await page.getByRole('button', { name: new RegExp(`Verify the browser workflow.*${state}`) }).click()
    await expect(page.getByLabel('Run detail surface')).toBeVisible()
    await scan(page, `run-detail-${state.toLowerCase()}`)
  }
  for (const [name, stageState, gateState, execution] of [
    ['skipped', 'SKIPPED', 'BLOCKED', 'LIVE'],
    ['not-run', 'SKIPPED', 'NOT_RUN', 'LIVE'],
    ['simulated', 'SUCCEEDED', 'PASS', 'SIMULATED'],
  ]) {
    const detailRun = { ...run, state: 'COMPLETED', execution }
    options.runs = [detailRun]
    options.report = { ...report, run: detailRun, stages: report.stages.map((stage, index) => ({ ...stage, state: index === 0 ? stageState : 'SUCCEEDED' })), gates: report.gates.map((gate, index) => ({ ...gate, verdict: index === 0 ? gateState : gate.verdict })) }
    await page.reload()
    await page.getByRole('button', { name: /Verify the browser workflow.*COMPLETED/ }).click()
    await scan(page, `run-detail-${name}`)
  }

  await page.reload()
  await page.getByRole('button', { name: /Route preview/ }).click()
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toBeVisible()
  await scan(page, 'route-preview-ready')
  await page.getByLabel('Project path').fill('./missing-project')
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await scan(page, 'route-preview-blocked')
  const disconnectedPreview = await context.newPage()
  await open(disconnectedPreview, { versionError: true })
  await disconnectedPreview.getByRole('button', { name: /Route preview/ }).click()
  await scan(disconnectedPreview, 'route-preview-disconnected')
  await disconnectedPreview.close()

  options.profiles = [profile]
  options.error = undefined
  await page.reload()
  await page.getByRole('button', { name: /Profile readiness/ }).click()
  await scan(page, 'profile-readiness-ready-matrix')
  const disconnectedReadiness = await context.newPage()
  await open(disconnectedReadiness, { versionError: true })
  await disconnectedReadiness.getByRole('button', { name: /Profile readiness/ }).click()
  await scan(disconnectedReadiness, 'profile-readiness-disconnected')
  await disconnectedReadiness.close()
  const errorReadiness = await context.newPage()
  await open(errorReadiness, { error: 'profiles' })
  await errorReadiness.getByRole('button', { name: /Profile readiness/ }).click()
  await scan(errorReadiness, 'profile-readiness-error')
  await errorReadiness.close()

  const emptySurface = await context.newPage()
  await open(emptySurface, { runs: [] })
  await scan(emptySurface, 'shared-empty-surface')
  await emptySurface.close()
  const errorSurface = await context.newPage()
  await open(errorSurface, { error: 'profiles' })
  await errorSurface.getByRole('button', { name: /Profiles & models/ }).click()
  await scan(errorSurface, 'shared-error-surface')
  await errorSurface.close()
  const disconnectedSurface = await context.newPage()
  await open(disconnectedSurface, { versionError: true })
  await scan(disconnectedSurface, 'shared-disconnected-surface')
  await disconnectedSurface.close()
})
