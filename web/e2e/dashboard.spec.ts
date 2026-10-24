import AxeBuilder from '@axe-core/playwright'
import { expect, test, type Page, type Route } from '@playwright/test'
import { mkdir } from 'node:fs/promises'
import { resolve } from 'node:path'

const screenshotDir = resolve(import.meta.dirname, '../../docs/screenshots')
const baselineDir = resolve(import.meta.dirname, '../../.taste-skill/before')
const stageKeys = ['constitution', 'specify', 'clarify', 'plan', 'checklist', 'tasks', 'analyze', 'implement', 'tests', 'converge', 'final_review', 'report']
const run = {
  id: 'browser-run-001',
  project_id: 'browser-project-001',
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

type FixtureOptions = { runs?: unknown[]; profiles?: unknown[]; error?: string; versionError?: boolean; report?: unknown; previewError?: string; previewBlocker?: Record<string, unknown>; previewDelayMs?: number; previewTransportFailure?: boolean; projectUnavailable?: boolean }

const projectFixture = { id: 'browser-project-001', canonical_path: '/workspace/demo', display_name: 'Browser demo', readiness: { status: 'READY', reason: 'Fixture configuration is ready.' } }
const secondProjectFixture = { id: 'browser-project-002', canonical_path: '/workspace/second', display_name: 'Second project', readiness: { status: 'READY', reason: 'Fixture configuration is ready.' } }

async function selectProject(page: Page) {
  await page.locator('#project-scope-select').selectOption(projectFixture.id)
}

async function installAPI(page: Page, options: FixtureOptions = {}) {
  const projects = [projectFixture, secondProjectFixture]
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
    if (path === '/api/v1/projects' && route.request().method() === 'POST') {
      const added = { id: 'browser-project-003', canonical_path: '/workspace/third', display_name: 'Third project', readiness: { status: 'MISSING_CONFIGURATION', reason: 'No configuration.' } }
      projects.push(added)
      return route.fulfill({ status: 201, contentType: 'application/json', body: JSON.stringify(added) })
    }
    if (path === '/api/v1/projects') return route.fulfill({ contentType: 'application/json', body: JSON.stringify(options.projectUnavailable ? [] : projects) })
    if (path === '/api/v1/workspace/status') return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ workspace_id: 'browser-workspace-001', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true }) })
    if (path === '/api/v1/runs' && route.request().method() === 'POST') {
      const body = route.request().postDataJSON() as { dry_run?: boolean; project_id?: string }
      if (body.dry_run) {
        if (options.previewTransportFailure) return route.abort('failed')
        if (options.previewDelayMs) await new Promise((resolveDelay) => setTimeout(resolveDelay, options.previewDelayMs))
        if (options.previewError) return route.fulfill({ status: 422, contentType: 'application/json', body: JSON.stringify({ error: { code: 'ROUTE_PREVIEW_BLOCKED', message: options.previewError, request_id: 'internal-support-id' }, request_id: 'internal-support-id', execution: 'NOT_RUN', dry_run: true, preview_blockers: [options.previewBlocker ?? { category: 'UNKNOWN', message: 'The route preview could not be completed. Review project readiness before trying again.', remediation: { target: 'readiness', label: 'Review project readiness' } }] }) })
        return route.fulfill({ contentType: 'application/json', body: JSON.stringify(readyPreview) })
      }
    const started = { ...run, id: 'started-browser-run', project_id: body.project_id, prompt: route.request().postDataJSON().prompt, state: 'RUNNING' }
    options.runs = [started, ...(options.runs ?? [])]
    return route.fulfill({ status: 202, contentType: 'application/json', body: JSON.stringify(started) })
    }
    if (path === '/api/v1/runs') {
      if (options.error === 'runs') return route.fulfill({ status: 503, contentType: 'application/json', body: '{"error":"run store unavailable"}' })
      const items = options.runs ?? [run]
      const projectID = url.searchParams.get('project_id')
      return route.fulfill({ contentType: 'application/json', headers: { 'X-Total-Count': String(projectID ? items.filter((item) => (item as typeof run).project_id === projectID).length : items.length) }, body: JSON.stringify(projectID ? items.filter((item) => (item as typeof run).project_id === projectID) : items) })
    }
    if (/^\/api\/v1\/runs\/[^/]+\/events$/.test(path)) {
      const runID = path.split('/')[4]
      const event = { run_id: runID, seq: 1, type: 'provider.event', stage: 'implement', payload: { summary: `${runID} fixture activity` } }
      return route.fulfill({ status: 200, contentType: 'text/event-stream; charset=utf-8', body: `event: provider.event\ndata: ${JSON.stringify(event)}\n\n` })
    }
    if (/^\/api\/v1\/runs\/[^/]+$/.test(path)) {
      const runID = path.split('/')[4]
      const selected = (options.runs ?? [run]).find((item) => (item as typeof run).id === runID) as typeof run | undefined
      return route.fulfill({ contentType: 'application/json', body: JSON.stringify({ ...(options.report ?? report), run: selected ?? run }) })
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

test('capture workflow guidance baseline at desktop, tablet, and phone widths', async ({ page }) => {
  await open(page)
  await selectProject(page)
  await page.getByRole('button', { name: /Workflow control/ }).click()
  await mkdir(baselineDir, { recursive: true })
  for (const width of [1440, 768, 390]) {
    await page.setViewportSize({ width, height: 1000 })
    await page.screenshot({ path: resolve(baselineDir, `workflow-${width}.png`), fullPage: true })
  }
})

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

test('project scope filters by stable ID and adds a third project from one absolute path', async ({ page }) => {
  const runA = { ...run, id: 'project-a-run', project_id: projectFixture.id, prompt: 'Project A workflow' }
  const runB = { ...run, id: 'project-b-run', project_id: secondProjectFixture.id, prompt: 'Project B workflow' }
  await open(page, { runs: [runA, runB] })
  await expect(page.getByText('Project A workflow')).toBeVisible()
  await expect(page.getByText('Project B workflow')).toBeVisible()
  await selectProject(page)
  await expect(page.getByText('Project A workflow')).toBeVisible()
  await expect(page.getByText('Project B workflow')).toHaveCount(0)
  await page.getByRole('button', { name: /Project A workflow/ }).click()
  await expect(page.getByText('project-a-run fixture activity')).toBeVisible()
  await expect(page.getByText('project-b-run fixture activity')).toHaveCount(0)
  await page.getByText('Add or switch registered projects').click()
  await page.locator('#add-project-path').focus()
  await page.keyboard.type('/workspace/third')
  await page.keyboard.press('Tab')
  await page.keyboard.press('Enter')
  await expect(page.locator('#project-scope-select')).toContainText('Third project')
  await expect(page.getByText('No configuration.')).toBeVisible()
})

test('empty, failed, and disconnected dashboard surfaces are labeled and accessible', async ({ page }) => {
  await open(page, { runs: [], previewError: 'PROJECT_PATH_UNAVAILABLE' })
  await expect(page.getByText('Select a run to inspect its stages, routes, and evidence.')).toBeVisible()
  await saveScreenshot(page, 'workflow-empty-desktop')
  await scan(page, 'empty-workflow-detail')
  await page.getByRole('button', { name: /Run history/ }).click()
  await expect(page.getByText('No runs recorded yet')).toBeVisible()
  await scan(page, 'empty-run-list')

  await page.getByRole('button', { name: /Workflow control/ }).click()
  await selectProject(page)
  await page.locator('#task-draft').fill('Check the current route')
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await expect(page.getByRole('alert', { name: 'Route preview blocked' })).toContainText('NOT_RUN')
  await expect(page.getByRole('alert', { name: 'Route preview blocked' })).not.toContainText('PROJECT_PATH_UNAVAILABLE')
  await expect(page.getByRole('alert', { name: 'Route preview blocked' })).not.toContainText('internal-support-id')
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
  await expect(profileErrorPage.getByRole('alert')).toContainText('Request failed (503)')
  await scan(profileErrorPage, 'profile-error')
  await profileErrorPage.close()
})

test('ready route preview, profile empty states, and profile readiness are accessible', async ({ page }) => {
  await open(page)
  await selectProject(page)
  await page.locator('#task-draft').fill('Verify the configured stage routes')
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toContainText('NOT_RUN')
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toContainText('No workflow or repository transfer occurred.')
  await expect(page.locator('.route-preview-results .route-list > li')).toHaveCount(12)
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

test('one selected project and draft carry through preview to an explicit start', async ({ page }) => {
  await open(page, { runs: [] })
  await selectProject(page)
  await page.locator('#task-draft').fill('Implement the selected project request')
  await page.getByRole('button', { name: 'Preview routes' }).click()
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toContainText('NOT_RUN')
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toContainText('No workflow or repository transfer occurred.')
  await page.getByRole('button', { name: 'Start workflow' }).click()
  await expect(page.locator('.status-line')).toContainText('1 active')
  await expect(page.getByRole('button', { name: /Implement the selected project request.*RUNNING/ })).toBeVisible()
  await expect(page.locator('.status-line')).not.toContainText('Unable to start')
})

test('pending preview results are discarded when the selected project changes', async ({ page }) => {
  await open(page, { runs: [], previewDelayMs: 900 })
  await selectProject(page)
  await page.locator('#task-draft').fill('Preview the first project')
  await page.getByRole('button', { name: 'Preview routes' }).click()
  await page.locator('#project-scope-select').selectOption(secondProjectFixture.id)
  await expect(page.locator('#task-draft')).toHaveValue('')
  await page.waitForTimeout(1000)
  await page.getByRole('button', { name: 'Route preview' }).click()
  await expect(page.getByText('Route preview has not been run. No work starts from this screen.')).toBeVisible()
  await expect(page.getByText(/browser-fixture.*deterministic/)).toHaveCount(0)
})

test('an unavailable active project keeps its task draft and disables preview actions', async ({ page }) => {
  const options: FixtureOptions = { runs: [] }
  await open(page, options)
  await selectProject(page)
  await page.locator('#task-draft').fill('Keep this unsent request in memory')
  options.projectUnavailable = true
  await page.getByRole('button', { name: 'Refresh project state' }).click()
  await expect(page.locator('#project-scope-select')).toHaveValue('', { timeout: 7000 })
  await expect(page.locator('#task-draft')).toHaveValue('Keep this unsent request in memory')
  await page.getByRole('button', { name: 'Route preview' }).click()
  await expect(page.getByRole('button', { name: 'Preview routes' })).toBeDisabled()
  await page.getByRole('button', { name: 'Workflow control' }).click()
  await expect(page.locator('#task-draft')).toHaveValue('Keep this unsent request in memory')
})

test('blocked previews show bounded recovery for every blocker category', async ({ page }) => {
  const cases = [
    { category: 'MISSING_ROUTE', target: 'routing', label: 'Review route settings', message: 'Routes are not configured for this project.' },
    { category: 'MISSING_PROFILE', target: 'profiles', label: 'Review profile setup', message: 'Profile browser-fixture is needed for the constitution stage.', stage: 'constitution', profile: 'browser-fixture', provider: 'codex' },
    { category: 'PROFILE_NOT_AUTHORIZED', target: 'profiles', label: 'Review profile access', message: 'The selected profile is not authorized for this project.' },
    { category: 'CAPABILITY_NOT_VERIFIED', target: 'readiness', label: 'Review profile readiness', message: 'Provider capability evidence needs verification.' },
    { category: 'UNKNOWN', target: 'readiness', label: 'Review project readiness', message: 'The route preview could not be completed safely.' },
  ]
  for (const item of cases) {
    const recoveryPage = await page.context().newPage()
    await open(recoveryPage, { previewError: 'raw-internal-diagnostic /private/repo', previewBlocker: { category: item.category, message: item.message, remediation: { target: item.target, label: item.label }, ...(item.stage ? { stage: item.stage, profile: item.profile, provider: item.provider } : {}) } })
    await selectProject(recoveryPage)
    await recoveryPage.locator('#task-draft').fill('Check blocked routes')
    await recoveryPage.getByRole('button', { name: /Preview routes/ }).click()
    const recovery = recoveryPage.getByRole('alert', { name: 'Route preview blocked' })
    await expect(recovery).toContainText('NOT_RUN')
    await expect(recovery).toContainText(item.message)
    await expect(recovery).not.toContainText('raw-internal-diagnostic')
    await expect(recovery).not.toContainText('internal-support-id')
    await recoveryPage.getByRole('button', { name: item.label }).click()
    await expect(recoveryPage.locator('h1')).toHaveText(item.target === 'profiles' ? 'Profiles & models' : item.target === 'readiness' ? 'Profile readiness' : 'Route preview')
    await scan(recoveryPage, `preview-${item.category.toLowerCase()}`)
    await recoveryPage.close()
  }
})

test('route-preview transport failure is disconnected and preserves project and task context', async ({ page }) => {
  await open(page, { previewTransportFailure: true })
  await selectProject(page)
  await page.locator('#task-draft').fill('Keep this task for retry')
  await page.getByRole('button', { name: /Preview routes/ }).click()
  const disconnected = page.locator('.route-preview-panel [aria-label="disconnected state"]')
  await expect(disconnected).toContainText('could not be reached')
  await expect(disconnected).toContainText('task draft are still here')
  await scan(page, 'route-preview-transport-disconnected')
  await page.getByRole('button', { name: /Workflow control/ }).click()
  await expect(page.locator('#project-scope-select')).toHaveValue(projectFixture.id)
  await expect(page.locator('#task-draft')).toHaveValue('Keep this task for retry')
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
  await page.getByRole('button', { name: /Workflow control/ }).click()
  await selectProject(page)
  await page.locator('#task-draft').fill('Check the selected project route')
  await page.getByRole('button', { name: /Route preview/ }).click()
  await page.getByRole('button', { name: /Preview routes/ }).click()
  await expect(page.getByRole('status', { name: 'Ready route preview' })).toBeVisible()
  await scan(page, 'route-preview-ready')
  options.previewError = 'PROJECT_PATH_UNAVAILABLE'
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
