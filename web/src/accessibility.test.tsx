// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import axe from 'axe-core'
import { App, ApprovalActions, EventList, ExecutionHistoryPanel, ProfileReadinessPanel, RoutePreviewPanel, RunDetailStatus, RunList, SharedStateSurface } from './App'
import type { Profile, Report, Run, RunState } from './domain'

afterEach(() => {
  cleanup()
  vi.unstubAllGlobals()
})

const run = (state: RunState, id: string = state): Run => ({
  id,
  project: '/workspace/demo',
  prompt: 'Review a bounded workflow',
  state,
  created_at: '2026-10-22T12:00:00Z',
})

const profile: Profile = {
  id: 'profile-1', name: 'local-codex', provider: 'codex', executable: 'codex', allowed_scopes: ['/workspace'],
  capability: { provider: 'codex', executable_found: true, version: '1.2.3', authenticated: 'AUTHENTICATED', read_only_execution: true, workspace_write: true },
}

const preview = {
  dry_run: true,
  execution: 'NOT_RUN',
  mode: 'hybrid',
  decisions: [{ stage: 'implement', route: { provider: 'fake', profile: 'offline', model: 'fixture-model', effort: 'high' }, explanation: 'deterministic offline route' }],
}
const projectFixture = { id: 'project-fixture', canonical_path: '/workspace/demo', display_name: 'demo', readiness: { status: 'READY', reason: 'Fixture project is ready.' } }
let savedProjectSelection = ''
const installProjectPreference = (value: string) => {
  savedProjectSelection = value
  vi.stubGlobal('localStorage', {
    getItem: () => savedProjectSelection,
    setItem: (_key: string, value: string) => { savedProjectSelection = value },
    removeItem: () => { savedProjectSelection = '' },
  })
}
const selectProjectFixture = () => installProjectPreference(projectFixture.id)

describe('FR-026 dashboard page and state matrix', () => {
  it('keeps all-project scope observational and shares one transient task draft', async () => {
    const second = { id: 'project-second', canonical_path: '/workspace/second', display_name: 'second', readiness: { status: 'READY', reason: 'Ready.' } }
    const requests: string[] = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input); requests.push(`${init?.method ?? 'GET'} ${url}`)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([projectFixture, second])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') || url.includes('/api/v1/runs?project_id=')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    installProjectPreference('')
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    expect((document.querySelector('#project-scope-select') as HTMLSelectElement).value).toBe('')
    expect(screen.getByRole('button', { name: 'Select a project' })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Preview routes' })).toBeNull()
    fireEvent.click(screen.getByRole('button', { name: 'Route preview' }))
    expect(screen.getByRole('button', { name: 'Preview routes' }).hasAttribute('disabled')).toBe(true)
    expect(requests.some((request) => request.startsWith('POST '))).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Offline demonstration · simulated' }))
    expect(await screen.findByRole('region', { name: 'Offline demonstration' })).toBeTruthy()
    expect(screen.getByText('Simulated · no API request')).toBeTruthy()
    expect(requests.some((request) => request.startsWith('POST '))).toBe(false)
    fireEvent.click(screen.getByRole('button', { name: 'Workflow control' }))
    fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: projectFixture.id } })
    fireEvent.change(screen.getByLabelText('Task prompt'), { target: { value: 'Keep this only for the current project' } })
    expect(screen.getAllByLabelText('Task prompt')).toHaveLength(1)
    expect(screen.queryByLabelText('Project path')).toBeNull()
    fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: second.id } })
    expect((screen.getByLabelText('Task prompt') as HTMLTextAreaElement).value).toBe('')
    expect(screen.queryByText(/Route preview has not been run/)).toBeNull()
    expect(requests.some((request) => request.startsWith('POST '))).toBe(false)
  })

  it('defaults to All projects, filters server activity by project ID, and adds only an absolute path', async () => {
    const second = { id: 'project-second', canonical_path: '/workspace/second', display_name: 'second' }
    const third = { id: 'project-third', canonical_path: '/workspace/third', display_name: 'third' }
    const firstRun = { ...run('COMPLETED', 'run-first'), prompt: 'first project workflow', project_id: projectFixture.id }
    const secondRun = { ...run('COMPLETED', 'run-second'), prompt: 'second project workflow', project_id: second.id }
    const requests: Array<{ url: string; body?: string }> = []
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input); requests.push({ url, body: init?.body?.toString() })
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects') && init?.method === 'POST') return json(third, 201)
      if (url.endsWith('/api/v1/projects')) return json([projectFixture, second])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.includes('/api/v1/runs?project_id=')) return json(url.includes(projectFixture.id) ? [firstRun] : [secondRun])
      if (url.endsWith('/api/v1/runs')) return json([firstRun, secondRun])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    installProjectPreference('')
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    expect((document.querySelector('#project-scope-select') as HTMLSelectElement).value).toBe('')
    expect(await screen.findByText('first project workflow')).toBeTruthy()
    expect(screen.getByText('second project workflow')).toBeTruthy()
    fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: projectFixture.id } })
    expect(await screen.findByText('first project workflow')).toBeTruthy()
    await waitFor(() => expect(requests.some((request) => request.url.includes(`project_id=${projectFixture.id}`))).toBe(true))
    await waitFor(() => expect(screen.queryByText('second project workflow')).toBeNull())
    fireEvent.change(document.querySelector('#add-project-path')!, { target: { value: '/workspace/third' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add project' }))
    await screen.findByRole('option', { name: /third/ })
    expect(JSON.parse(requests.find((request) => request.url.endsWith('/api/v1/projects') && request.body)?.body ?? '{}')).toEqual({ path: '/workspace/third' })
  })

  it.each(['unknown-project-id', '{malformed-selection'])('clears stale saved project selection %s and falls back to All projects', async (saved) => {
    installProjectPreference(saved)
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([projectFixture])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    await waitFor(() => expect((document.querySelector('#project-scope-select') as HTMLSelectElement).value).toBe(''))
    expect(savedProjectSelection).toBe('')
  })

  it('shows the selected project readiness and routes direct actions to configuration or profiles', async () => {
    const second = { id: 'project-needs-profile', canonical_path: '/workspace/other', display_name: 'other', readiness: { status: 'PROFILE_NOT_AUTHORIZED', reason: 'A provider profile needs authorization.', action: 'Review the local provider profile.' } }
    const first = { ...projectFixture, readiness: { status: 'MISSING_ROUTE', reason: 'No default route is configured.', action: 'Configure a route for this project.' } }
    installProjectPreference('')
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([first, second])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') || url.includes('/api/v1/runs?project_id=')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: projectFixture.id } })
    expect(await screen.findByText('No default route is configured.')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /Review route settings/ }))
    expect(screen.getByRole('heading', { name: 'Route preview' })).toBeTruthy()
    fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: second.id } })
    fireEvent.click(screen.getByRole('button', { name: /Workflow control/ }))
    expect(await screen.findByText('A provider profile needs authorization.')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /Review profile access/ }))
    expect(screen.getByRole('heading', { name: 'Profiles & models' })).toBeTruthy()
  })

  it('announces rejected add-project paths in the labeled form', async () => {
    installProjectPreference('')
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects') && init?.method === 'POST') return json({ error: { code: 'PROJECT_INVALID', message: 'Project path is not an accessible directory.' } }, 422)
      if (url.endsWith('/api/v1/projects')) return json([projectFixture])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    fireEvent.change(document.querySelector('#add-project-path')!, { target: { value: '/workspace/missing' } })
    fireEvent.click(screen.getByRole('button', { name: 'Add project' }))
    expect(await screen.findByRole('alert')).toBeTruthy()
    expect(document.querySelector('#add-project-path')?.getAttribute('aria-describedby')).toBe('add-project-error')
  })

  it.each(['RUNNING', 'PAUSED', 'BLOCKED', 'COMPLETED', 'CANCELLED', 'FAILED'] as RunState[])(
    'labels %s in the run-list surface', (state) => {
      render(<RunList runs={[run(state)]} selected="" onSelect={() => undefined} />)
      expect(screen.getByRole('button', { name: new RegExp(state) })).toBeTruthy()
    },
  )

  it('renders the empty run-list state with text', () => {
    render(<RunList runs={[]} selected="" onSelect={() => undefined} />)
    expect(screen.getByRole('status', { name: 'Empty run list' }).textContent).toContain('No runs recorded yet')
  })

  it('labels each history row from its immutable execution class without selecting it', () => {
    const runs: Run[] = [
      { ...run('COMPLETED', 'run-simulated'), execution: 'SIMULATED' },
      { ...run('COMPLETED', 'run-live'), execution: 'LIVE' },
      { ...run('BLOCKED', 'run-mixed'), execution: 'MIXED' },
      { ...run('FAILED', 'run-unknown'), execution: 'UNKNOWN' },
    ]
    render(<RunList runs={runs} selected="" onSelect={() => undefined} />)
    for (const execution of ['SIMULATED', 'LIVE', 'MIXED', 'UNKNOWN']) expect(screen.getByText(execution)).toBeTruthy()
    expect(screen.getAllByText('COMPLETED')).toHaveLength(2)
  })

  it('labels a disconnected run list while preserving its rows', () => {
    render(<RunList runs={[run('RUNNING')]} selected="" onSelect={() => undefined} disconnected />)
    expect(screen.getByRole('status').textContent).toContain('Disconnected')
    expect(screen.getByRole('button', { name: /RUNNING/ })).toBeTruthy()
  })

  it.each(['CREATED', 'RUNNING', 'PAUSED', 'BLOCKED', 'COMPLETED', 'CANCELLED', 'FAILED'] as RunState[])(
    'renders the %s run-detail state with the correct available control', (state) => {
      const pause = vi.fn()
      const resume = vi.fn()
      const cancel = vi.fn()
      render(<RunDetailStatus run={run(state)} onPause={pause} onResume={resume} onCancel={cancel} />)
      expect(screen.getByText(state)).toBeTruthy()
      if (state === 'RUNNING') {
        fireEvent.click(screen.getByRole('button', { name: 'Pause' }))
        fireEvent.click(screen.getByRole('button', { name: 'Cancel run' }))
        expect(pause).toHaveBeenCalledOnce()
        expect(cancel).toHaveBeenCalledOnce()
      } else if (state === 'PAUSED') {
        fireEvent.click(screen.getByRole('button', { name: 'Resume' }))
        expect(resume).toHaveBeenCalledOnce()
      } else {
        expect(screen.queryByRole('button')).toBeNull()
      }
    },
  )

  it('renders route-preview ready, blocked, and disconnected states', () => {
    const remedy = vi.fn()
    const { rerender } = render(<RoutePreviewPanel state="ready" preview={preview} blocker={null} error="" onRemediate={remedy} />)
    expect(screen.getByText(/Ready · NOT_RUN/)).toBeTruthy()
    expect(screen.getByText(/fixture-model.*high/)).toBeTruthy()
    expect(screen.getByRole('list', { name: 'Stage route decisions' })).toBeTruthy()
    expect(screen.getByRole('heading', { name: 'implement' })).toBeTruthy()
    rerender(<RoutePreviewPanel state="blocked" preview={null} blocker={{ category: 'MISSING_PROFILE', message: 'Profile local-codex is needed for the constitution stage.', remediation: { target: 'profiles', label: 'Review profile setup' }, stage: 'constitution', profile: 'local-codex', provider: 'codex' }} error="" onRemediate={remedy} />)
    expect(screen.getByRole('alert', { name: 'Route preview blocked' }).textContent).toContain('NOT_RUN')
    expect(screen.getByRole('alert', { name: 'Route preview blocked' }).textContent).toContain('constitution')
    fireEvent.click(screen.getByRole('button', { name: 'Review profile setup' }))
    expect(remedy).toHaveBeenCalledWith('profiles')
    rerender(<RoutePreviewPanel state="blocked" preview={null} blocker={{ category: 'CAPABILITY_NOT_VERIFIED', message: 'Provider authentication evidence needs verification.', remediation: { target: 'readiness', label: 'Review profile readiness' } }} error="" onRemediate={remedy} />)
    fireEvent.click(screen.getByRole('button', { name: 'Review profile readiness' }))
    expect(remedy).toHaveBeenLastCalledWith('readiness')
    rerender(<RoutePreviewPanel state="disconnected" preview={null} blocker={null} error="API unavailable" onRemediate={remedy} />)
    expect(screen.getByRole('status').textContent).toContain('Disconnected')
  })

  it('renders profile-readiness ready, disconnected, and error states', () => {
    const { rerender } = render(<ProfileReadinessPanel profiles={[profile]} disconnected={false} error="" />)
    expect(screen.getByText('READY')).toBeTruthy()
    rerender(<ProfileReadinessPanel profiles={[]} disconnected error="" />)
    expect(screen.getByRole('status').textContent).toContain('Disconnected')
    rerender(<ProfileReadinessPanel profiles={[]} disconnected={false} error="STORE_ERROR" />)
    expect(screen.getByRole('alert').textContent).toContain('STORE_ERROR')
  })

  it('opens the actual route-preview view and sends a dry-run request', async () => {
    let requestBody: Record<string, unknown> | undefined
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([projectFixture])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') && init?.method === 'POST') {
        requestBody = JSON.parse(String(init.body)) as Record<string, unknown>
        return json(preview)
      }
      if (url.endsWith('/api/v1/runs') || url.includes('/api/v1/runs?project_id=')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    Object.defineProperty(globalThis.crypto, 'randomUUID', { configurable: true, value: () => 'preview-key-12345678' })
    selectProjectFixture()
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    fireEvent.change(screen.getByLabelText('Task prompt'), { target: { value: 'Check the selected project routes' } })
    fireEvent.click(await screen.findByRole('button', { name: /Preview routes/ }))
    expect(await screen.findByText(/Ready · NOT_RUN/)).toBeTruthy()
    expect(requestBody?.dry_run).toBe(true)
    expect(requestBody?.project_id).toBe(projectFixture.id)
  })

  it.each(['Project scope', 'Task prompt'])('ignores a route preview that resolves after %s changes', async (field) => {
    let resolvePreview!: (response: Response) => void
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([projectFixture])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') && init?.method === 'POST') return new Promise<Response>((resolve) => { resolvePreview = resolve })
      if (url.endsWith('/api/v1/runs') || url.includes('/api/v1/runs?project_id=')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    Object.defineProperty(globalThis.crypto, 'randomUUID', { configurable: true, value: () => 'preview-key-delayed' })
    selectProjectFixture()
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    fireEvent.change(screen.getByLabelText('Task prompt'), { target: { value: 'Check route state' } })
    fireEvent.click(await screen.findByRole('button', { name: /Preview routes/ }))
    if (field === 'Project scope') {
      const second = { id: 'project-second', canonical_path: '/workspace/second', display_name: 'second', readiness: { status: 'READY', reason: 'Ready.' } }
      fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: second.id } })
    } else fireEvent.change(screen.getByLabelText('Task prompt'), { target: { value: 'Changed intent while preview is pending' } })
    await act(async () => { resolvePreview(await json(preview)) })
    fireEvent.click(screen.getByRole('button', { name: /Route preview/ }))
    expect(screen.getByText(/Route preview has not been run/)).toBeTruthy()
    expect(screen.queryByText(/fixture-model/)).toBeNull()
  })

  it('keeps the selected run report when an older selection resolves late', async () => {
    const older: Report = { run: run('COMPLETED', 'run-a'), stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] }
    const newer: Report = { run: run('COMPLETED', 'run-b'), stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] }
    const pending = new Map<string, (response: Response) => void>()
    const runs: Run[] = [run('COMPLETED', 'run-a'), run('COMPLETED', 'run-b')]
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json(runs)
      if (url.includes('/api/v1/runs/run-a')) return new Promise<Response>((resolve) => { pending.set('run-a', resolve) })
      if (url.includes('/api/v1/runs/run-b')) return new Promise<Response>((resolve) => { pending.set('run-b', resolve) })
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await waitFor(() => expect(document.querySelectorAll('.run-card')).toHaveLength(2))
    const cards = [...document.querySelectorAll<HTMLButtonElement>('.run-card')]
    fireEvent.click(cards[0])
    fireEvent.click(cards[1])
    await act(async () => { pending.get('run-b')?.(await json(newer)) })
    expect(await screen.findByText('/workspace/demo')).toBeTruthy()
    await act(async () => { pending.get('run-a')?.(await json(older)) })
    expect(screen.getByText('run-b')).toBeTruthy()
  })

  it('ignores an older overlapping refresh after selection changes', async () => {
    const older: Report = { run: run('COMPLETED', 'run-a'), stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] }
    const newer: Report = { run: run('COMPLETED', 'run-b'), stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] }
    const pending = new Map<string, Array<(response: Response) => void>>()
    const runs: Run[] = [run('COMPLETED', 'run-a'), run('COMPLETED', 'run-b')]
    const streams: Array<{ onopen?: () => void; close(): void; addEventListener(): void }> = []
    vi.stubGlobal('fetch', vi.fn((input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json(runs)
      if (url.includes('/api/v1/runs/run-a')) return new Promise<Response>((resolve) => pending.set('run-a', [...(pending.get('run-a') ?? []), resolve]))
      if (url.includes('/api/v1/runs/run-b')) return new Promise<Response>((resolve) => pending.set('run-b', [...(pending.get('run-b') ?? []), resolve]))
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class {
      onopen?: () => void
      constructor() { streams.push(this) }
      close() {}
      addEventListener() {}
    })
    render(<App />)
    await waitFor(() => expect(document.querySelectorAll('.run-card')).toHaveLength(2))
    const cards = [...document.querySelectorAll<HTMLButtonElement>('.run-card')]
    fireEvent.click(cards[0])
    await waitFor(() => expect(pending.get('run-a')).toHaveLength(1))
    await act(async () => { streams[0].onopen?.() })
    await waitFor(() => expect(pending.get('run-a')).toHaveLength(2))
    fireEvent.click(cards[1])
    await waitFor(() => expect(pending.get('run-b')).toHaveLength(1))
    await act(async () => { pending.get('run-b')?.[0](await json(newer)) })
    expect(await screen.findByText('run-b')).toBeTruthy()
    await act(async () => { pending.get('run-a')?.[1](await json(older)) })
    await act(async () => { pending.get('run-a')?.[0](await json(older)) })
    expect(screen.getByText('run-b')).toBeTruthy()
  })

  it('renders the build version returned by the API', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture', version: '9.8.7-test' })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    expect(await screen.findByText('9.8.7-test')).toBeTruthy()
  })

  it('keeps profile views usable when the API returns a null empty list', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return json(null)
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: /Profile readiness/ }))
    expect(await screen.findByText('No profiles to assess')).toBeTruthy()
    fireEvent.click(screen.getByRole('button', { name: /Profiles & models/ }))
    expect(await screen.findByText('No profiles registered')).toBeTruthy()
  })

  it('invalidates a ready route preview when project intent changes', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([projectFixture])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') && init?.method === 'POST') return json(preview)
      if (url.endsWith('/api/v1/runs') || url.includes('/api/v1/runs?project_id=')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    Object.defineProperty(globalThis.crypto, 'randomUUID', { configurable: true, value: () => 'preview-key-12345678' })
    selectProjectFixture()
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    fireEvent.change(screen.getByLabelText('Task prompt'), { target: { value: 'Check route state' } })
    fireEvent.click(await screen.findByRole('button', { name: /Preview routes/ }))
    expect(await screen.findByText(/fixture-model.*high/)).toBeTruthy()
    fireEvent.change(document.querySelector('#project-scope-select')!, { target: { value: '' } })
    expect(screen.getByText(/Route preview has not been run/)).toBeTruthy()
    expect(screen.queryByText(/fixture-model/)).toBeNull()
  })

  it('shows a route-preview transport failure as disconnected and keeps the selected project and task', async () => {
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/projects')) return json([projectFixture])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') && init?.method === 'POST') throw new TypeError('Failed to fetch')
      if (url.endsWith('/api/v1/runs') || url.includes('/api/v1/runs?project_id=')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    Object.defineProperty(globalThis.crypto, 'randomUUID', { configurable: true, value: () => 'preview-key-12345678' })
    selectProjectFixture()
    render(<App />)
    await screen.findByRole('option', { name: /demo/ })
    fireEvent.change(screen.getByLabelText('Task prompt'), { target: { value: 'Keep this task for retry' } })
    fireEvent.click(screen.getByRole('button', { name: /Preview routes/ }))
    const previewDisconnected = await waitFor(() => {
      const panel = document.querySelector('.route-preview-panel [aria-label="disconnected state"]')
      if (!panel) throw new Error('route-preview disconnected surface not rendered')
      return panel
    })
    expect(previewDisconnected.textContent).toContain('could not be reached')
    fireEvent.click(screen.getByRole('button', { name: 'Workflow control' }))
    expect((screen.getByLabelText('Task prompt') as HTMLTextAreaElement).value).toBe('Keep this task for retry')
    expect((document.querySelector('#project-scope-select') as HTMLSelectElement).value).toBe(projectFixture.id)
  })

  it('refreshes disconnected state from an explicit retry without background polling', async () => {
    vi.useFakeTimers()
    let disconnected = true
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return disconnected ? Promise.reject(new Error('offline')) : json([])
      if (url.endsWith('/api/v1/runs')) return disconnected ? Promise.reject(new Error('offline')) : json([])
      if (url.endsWith('/api/v1/projects')) return json([])
      if (url.endsWith('/api/v1/workspace/status')) return json({ workspace_id: 'workspace-fixture', version: 'test', dashboard_url: 'http://127.0.0.1:4747', local_only: true })
      return json({}, 404)
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await act(async () => { await vi.advanceTimersByTimeAsync(0) })
    expect(screen.getAllByLabelText('disconnected state').length).toBeGreaterThan(0)
    const requestsWhileOffline = fetchMock.mock.calls.length
    disconnected = false
    await act(async () => { await vi.advanceTimersByTimeAsync(1800) })
    expect(fetchMock.mock.calls.length).toBe(requestsWhileOffline)
    expect(screen.getAllByLabelText('disconnected state').length).toBeGreaterThan(0)
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'Retry after reconnecting' }))
      await vi.advanceTimersByTimeAsync(0)
    })
    expect(fetchMock.mock.calls.length).toBeGreaterThan(requestsWhileOffline)
    vi.useRealTimers()
  })

  it('retries an unavailable initial version handshake until the API recovers', async () => {
    vi.useFakeTimers()
    let versionAttempts = 0
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) {
        versionAttempts++
        return versionAttempts === 1 ? json({ error: 'starting' }, 503) : json({ csrf_token: 'csrf-fixture' })
      }
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await act(async () => { await vi.advanceTimersByTimeAsync(0) })
    expect(versionAttempts).toBe(1)
    expect(screen.getAllByLabelText('disconnected state').length).toBeGreaterThan(0)
    await act(async () => { await vi.advanceTimersByTimeAsync(1800) })
    expect(versionAttempts).toBe(2)
    expect(screen.getByText(/0 active · 0 completed · 0 durable runs/)).toBeTruthy()
    expect(screen.queryByLabelText('disconnected state')).toBeNull()
    vi.useRealTimers()
  })

  it('keeps a selected event stream disconnected while run polling stays healthy', async () => {
    vi.useFakeTimers()
    const item = run('RUNNING', 'stream-run')
    class MockEventSource {
      static instances: MockEventSource[] = []
      onerror: (() => void) | null = null
      close() {}
      addEventListener() {}
      constructor() { MockEventSource.instances.push(this) }
    }
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([item])
      if (url.endsWith('/api/v1/runs/stream-run')) return json({ run: item, stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] })
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', MockEventSource)
    render(<App />)
    await act(async () => { await vi.advanceTimersByTimeAsync(0) })
    fireEvent.click(screen.getByRole('button', { name: /Review a bounded workflow.*RUNNING/ }))
    await act(async () => { await vi.advanceTimersByTimeAsync(0) })
    expect(MockEventSource.instances.length).toBeGreaterThan(0)
    await act(async () => { MockEventSource.instances[0].onerror?.() })
    expect(screen.getAllByText(/Reconnecting/).length).toBeGreaterThan(0)
    await act(async () => { await vi.advanceTimersByTimeAsync(1800) })
    expect(screen.getAllByText(/Reconnecting/).length).toBeGreaterThan(0)
    vi.useRealTimers()
  })

  it('recovers CSRF mutations and the event cursor after daemon restart without reloading', async () => {
    const item = run('RUNNING', 'restart-run')
    const report: Report = {
      run: item,
      acceptance: { execution: 'SIMULATED', verdict: 'PASS', required_gates: [] },
      stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [],
    }
    let serverToken = 'csrf-before-restart'
    let versionRequests = 0
    let acceptedMutations = 0
    const mutationTokens: string[] = []
    class MockEventSource {
      static instances: MockEventSource[] = []
      onerror: (() => void) | null = null
      onopen: (() => void) | null = null
      listeners = new Map<string, (event: Event) => void>()
      constructor(readonly url: string) { MockEventSource.instances.push(this) }
      close() {}
      addEventListener(type: string, listener: EventListenerOrEventListenerObject) {
        if (typeof listener === 'function') this.listeners.set(type, listener)
      }
      emit(type: string, value: unknown) {
        this.listeners.get(type)?.(new MessageEvent(type, { data: JSON.stringify(value) }))
      }
    }
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) {
        versionRequests++
        return json({ csrf_token: serverToken })
      }
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([item])
      if (url.endsWith('/api/v1/runs/restart-run')) return json(report)
      if (url.endsWith('/api/v1/runs/restart-run/cancel') && init?.method === 'POST') {
        const requestToken = new Headers(init.headers).get('X-SpecTide-CSRF') ?? ''
        mutationTokens.push(requestToken)
        if (requestToken === serverToken) {
          acceptedMutations++
          return json({ accepted: true })
        }
        return json({ error: { code: 'CSRF_REQUIRED', message: 'missing or invalid CSRF token' } }, 403)
      }
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', MockEventSource)
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: /Review a bounded workflow.*RUNNING/ }))
    const cancel = await screen.findByRole('button', { name: 'Cancel run' })
    await waitFor(() => expect(MockEventSource.instances.length).toBe(1))

    // Simulate a restart between reads: the old mutation is rejected, refreshed
    // token is fetched, and the ambiguous request is not replayed.
    serverToken = 'csrf-after-restart'
    fireEvent.click(cancel)
    await waitFor(() => expect(versionRequests).toBe(2))
    await waitFor(() => expect(mutationTokens).toEqual(['csrf-before-restart']))

    // A recovered event stream refreshes the handshake too and preserves the
    // selected run's replay cursor across the recovery.
    MockEventSource.instances[0].emit('provider.event', { run_id: item.id, seq: 7, type: 'provider.event' })
    MockEventSource.instances[0].onerror?.()
    await waitFor(() => expect(MockEventSource.instances.length).toBe(2), { timeout: 1500 })
    MockEventSource.instances[1].onopen?.()
    await waitFor(() => expect(versionRequests).toBe(3))
    expect(MockEventSource.instances[1].url).toContain('after=7')

    fireEvent.click(cancel)
    await waitFor(() => expect(mutationTokens).toEqual(['csrf-before-restart', 'csrf-after-restart']))
    await waitFor(() => expect(acceptedMutations).toBe(1))
    expect(screen.getByRole('button', { name: /Review a bounded workflow.*RUNNING/ })).toBeTruthy()
  })

  it.each([
    ['empty', 'No records yet'],
    ['error', 'The stored report could not be loaded'],
    ['disconnected', 'The local API cannot be reached'],
  ] as const)('renders the shared %s state', (state, message) => {
    render(<SharedStateSurface state={state} message={message} />)
    expect(screen.getByText(message)).toBeTruthy()
  })

  it('shows the persisted active simulated stage, route, and attempt', async () => {
    const item: Run = { ...run('RUNNING', 'active-simulated-run'), execution: 'SIMULATED' }
    const report: Report = {
      run: item,
      acceptance: { execution: 'SIMULATED', verdict: 'NOT_RUN', required_gates: [] },
      stages: [{ id: 'active-stage-1', key: 'constitution', state: 'RUNNING', provider: 'fake', profile: 'offline', model: 'fixture-model', effort: 'high', attempt: 1 }],
      gates: [], routing_decisions: [{ stage: 'constitution', provider: 'fake', profile: 'offline', model: 'fixture-model', effort: 'high', policy: 'simulated', explanation: 'offline deterministic fixture' }],
      artifacts: [], approvals: [],
    }
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([item])
      if (url.endsWith('/api/v1/runs/active-simulated-run')) return json(report)
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: /Review a bounded workflow.*RUNNING/ }))
    const detail = await screen.findByLabelText('Run detail surface')
    await waitFor(() => expect(detail.querySelector('.stage-row')).not.toBeNull())
    const row = detail.querySelector('.stage-row')
    expect(row).not.toBeNull()
    expect(within(row as HTMLElement).getByText('RUNNING')).toBeTruthy()
    expect(row?.textContent).toContain('fake / offline')
    expect(row?.textContent).toContain('fixture-model')
    expect(row?.textContent).toContain('high')
    expect(row?.textContent).toContain('attempt 1')
  })

  it('opens a persisted run detail and exposes stage, gate, and run states', async () => {
    const item = run('BLOCKED', 'run-1')
    const report: Report = {
      run: item,
      acceptance: { execution: 'MIXED', verdict: 'BLOCKED', required_gates: [] },
      stages: [{ id: 'stage-1', key: 'analyze', state: 'SKIPPED', provider: 'fake', profile: 'offline', model: 'fixture-model', effort: 'high', attempt: 1 }],
      execution_attempts: [{ id: 'attempt-2', stage: 'analyze', phase: 'analyze_remediation', iteration: 2, provider: 'fake', profile: 'offline', model: 'fixture-model', effort: 'high', started_at: '2026-10-22T12:00:00Z', result: 'RUNNING', duration_ms: 0, exit_code: 0 }],
      quality_executions: [{ id: 'quality-1', phase: 'tests', iteration: 2, started_at: '2026-10-22T12:00:00Z', finished_at: '2026-10-22T12:00:02Z', verdict: 'PASS', results: [{ key: 'unit_tests', verdict: 'PASS', command: ['go', 'test', './...'], duration_ns: 2000000000, summary: 'all checks passed', exit_code: 0 }] }],
      gates: [{ key: 'required_tests', verdict: 'NOT_RUN', evidence: 'blocked before tests' }],
      routing_decisions: [{ stage: 'analyze', provider: 'fake', profile: 'offline', model: '', effort: '', policy: 'hybrid', explanation: 'offline fixture' }], artifacts: [], approvals: [],
    }
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/runs')) return json([item])
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs/run-1')) return json(report)
      return json({}, 404)
    })
    vi.stubGlobal('fetch', fetchMock)
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    const runButton = await screen.findByRole('button', { name: /Review a bounded workflow.*BLOCKED/ })
    fireEvent.click(runButton)
    const detail = await screen.findByLabelText('Run detail surface')
    expect(within(detail).getByText('SKIPPED')).toBeTruthy()
    expect(screen.getByText('NOT_RUN')).toBeTruthy()
    expect(screen.getByText('MIXED')).toBeTruthy()
    expect(screen.getAllByText('BLOCKED').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText(/fixture-model.*high.*analyze_remediation iteration 2/)).toBeTruthy()
    expect(screen.getByText(/analyze_remediation · ACTIVE/)).toBeTruthy()
    expect(screen.getByText(/tests · iteration 2 · PASS/)).toBeTruthy()
    expect(screen.getByText(/unit_tests: PASS · all checks passed/)).toBeTruthy()
    expect(screen.getAllByText('BLOCKED').length).toBeGreaterThanOrEqual(1)
    expect(screen.getByText('MIXED')).toBeTruthy()
  })

  it('replays ordered missed events to two viewers and refreshes both canonical run snapshots', async () => {
    const item = run('COMPLETED', 'shared-run')
    let snapshot: Report = { run: item, stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] }
    class MockEventSource {
      static instances: MockEventSource[] = []
      onmessage: ((event: MessageEvent<string>) => void) | null = null
      onopen: (() => void) | null = null
      onerror: (() => void) | null = null
      listeners = new Map<string, (event: MessageEvent<string>) => void>()
      closed = false
      url: string
      constructor(url: string) { this.url = url; MockEventSource.instances.push(this) }
      addEventListener(type: string, listener: EventListenerOrEventListenerObject) { this.listeners.set(type, listener as (event: MessageEvent<string>) => void) }
      close() { this.closed = true }
      emit(seq: number, type: string) {
        const event = new MessageEvent(type, { data: JSON.stringify({ run_id: item.id, seq, type, stage: 'analyze' }) })
        this.listeners.get(type)?.(event)
      }
    }
    MockEventSource.instances = []
    vi.stubGlobal('EventSource', MockEventSource)
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs')) return json([item])
      if (url.endsWith('/api/v1/runs/shared-run')) return json(snapshot)
      return json({}, 404)
    }))
    const left = render(<App />)
    const right = render(<App />)
    await waitFor(() => expect(screen.getAllByRole('button', { name: /Review a bounded workflow.*COMPLETED/ })).toHaveLength(2))
    screen.getAllByRole('button', { name: /Review a bounded workflow.*COMPLETED/ }).forEach((button) => fireEvent.click(button))
    await waitFor(() => expect(MockEventSource.instances.length).toBeGreaterThanOrEqual(2))
    const originalStreams = MockEventSource.instances.slice(0, 2)
    await act(async () => originalStreams.forEach((stream) => stream.emit(1, 'provider.event')))
    await act(async () => { originalStreams.forEach((stream) => stream.onerror?.()) })
    expect(screen.getAllByText(/Reconnecting/).length).toBe(2)

    snapshot = {
      ...snapshot,
      run: { ...item, state: 'PAUSED' },
      gates: [{ key: 'required_tests', verdict: 'PASS', evidence: 'shared quality evidence' }],
      approvals: [{ id: 'approval-shared', action: 'Write project files', status: 'PENDING', decision: '' }],
    }
    await waitFor(() => expect(MockEventSource.instances.length).toBeGreaterThanOrEqual(4), { timeout: 2000 })
    const replayStreams = MockEventSource.instances.slice(2, 4)
    expect(replayStreams.map((stream) => stream.url)).toEqual([
      '/api/v1/runs/shared-run/events?after=1', '/api/v1/runs/shared-run/events?after=1',
    ])
    await act(async () => replayStreams.forEach((stream) => stream.onopen?.()))
    const backlog = [
      [2, 'approval.created'], [3, 'quality.gate_recorded'], [4, 'provider.codex.turn.started'], [5, 'provider.codex.turn.completed'],
    ] as const
    await act(async () => {
      replayStreams.forEach((stream) => {
        backlog.forEach(([seq, type]) => stream.emit(seq, type))
        stream.emit(3, 'quality.gate_recorded')
      })
    })

    await waitFor(() => {
      expect(screen.getAllByText('shared quality evidence').length).toBe(2)
      expect(screen.getAllByText('Write project files').length).toBe(2)
    })
    for (const container of [left.container, right.container]) {
      const rows = [...container.querySelectorAll('.event-row')]
      expect(rows.map((row) => row.textContent)).toEqual([
        expect.stringContaining('provider event'),
        expect.stringContaining('approval created'),
        expect.stringContaining('quality gate_recorded'),
        expect.stringContaining('provider codex turn started'),
        expect.stringContaining('provider codex turn completed'),
      ])
      expect(container.querySelectorAll('.event-row')).toHaveLength(5)
      expect(container.querySelector('[aria-label="Event stream connected"]')?.textContent).toContain('Live')
    }
    expect(replayStreams[0].url).toContain('after=1')
  })

  it('shows safe provider event summaries without rendering raw payload bodies', () => {
    render(<EventList events={[{ id: 'run-1-1', run_id: 'run-1', seq: 1, type: 'provider.event', stage: 'implement', payload: { provider_event_type: 'codex.turn.completed', summary: 'structured provider event', raw: 'private provider body' } }]} />)
    expect(screen.getByText('structured provider event')).toBeTruthy()
    expect(screen.getByText('codex.turn.completed')).toBeTruthy()
    expect(screen.queryByText('private provider body')).toBeNull()
  })

  it('renders persisted attempt iterations and quality outcomes, including the active invocation', () => {
    render(<ExecutionHistoryPanel attempts={[{ id: 'attempt-active', stage: 'implement', phase: 'convergence_implementation', iteration: 3, provider: 'codex', profile: 'work', model: 'model-a', effort: 'high', started_at: '2026-10-22T12:00:00Z', result: 'RUNNING', duration_ms: 0, exit_code: 0 }]} qualityExecutions={[{ id: 'quality-1', phase: 'tests', iteration: 2, started_at: '2026-10-22T12:00:00Z', finished_at: '2026-10-22T12:00:02Z', verdict: 'FAIL', results: [{ key: 'unit_tests', verdict: 'FAIL', command: ['go', 'test', './...'], duration_ns: 2000000000, summary: 'one package failed', exit_code: 1 }] }]} />)
    expect(screen.getByText(/convergence_implementation · ACTIVE/)).toBeTruthy()
    expect(screen.getByText(/codex \/ work · model-a · high · iteration 3/)).toBeTruthy()
    expect(screen.getByText(/tests · iteration 2 · FAIL/)).toBeTruthy()
    expect(screen.getByText(/unit_tests: FAIL · one package failed/)).toBeTruthy()
  })

  it('labels an omitted Codex model as the unreported CLI default', () => {
    render(<ExecutionHistoryPanel attempts={[{ id: 'attempt-codex-default', stage: 'clarify', phase: 'clarify', iteration: 1, provider: 'codex', profile: 'work', model: '', started_at: '2026-10-22T12:00:00Z', result: 'SUCCEEDED', duration_ms: 100, exit_code: 0 }]} qualityExecutions={[]} />)
    expect(screen.getByText(/CLI default \(not reported by provider\)/)).toBeTruthy()
    expect(screen.getByText(/Codex account usage is managed by the selected profile/)).toBeTruthy()
  })

  it('shows only the sanitized resolved Codex model and token counts when reported', () => {
    render(<ExecutionHistoryPanel attempts={[{ id: 'attempt-usage', stage: 'clarify', phase: 'clarify', iteration: 1, provider: 'codex', profile: 'work', model: '', started_at: '2026-10-22T12:00:00Z', result: 'SUCCEEDED', duration_ms: 100, exit_code: 0, evidence_json: JSON.stringify({ resolved_model: 'openai/gpt-5.2-codex', input_tokens: 42, output_tokens: 7, account_email: 'private@example.com' }) }]} qualityExecutions={[]} />)
    expect(screen.getByText(/openai\/gpt-5.2-codex/)).toBeTruthy()
    expect(screen.getByText(/42 in \/ 7 out tokens/)).toBeTruthy()
    expect(screen.queryByText('private@example.com')).toBeNull()
  })
})

describe('sensitive workspace approval controls', () => {
  it('offers keyboard-operable approve and deny controls only for a paused pending approval', () => {
    const onResolve = vi.fn()
    const approvals = [{ id: 'approval-1', action: 'Write project files during implement stage', status: 'PENDING', decision: '' }]
    const { rerender } = render(<ApprovalActions approvals={approvals} runState="PAUSED" onResolve={onResolve} />)
    const approve = screen.getByRole('button', { name: /approve write project files/i })
    const deny = screen.getByRole('button', { name: /deny write project files/i })
    fireEvent.click(approve)
    expect(onResolve).toHaveBeenCalledWith('approval-1', 'approved')
    fireEvent.click(deny)
    expect(onResolve).toHaveBeenLastCalledWith('approval-1', 'denied')
    rerender(<ApprovalActions approvals={approvals} runState="RUNNING" onResolve={onResolve} />)
    expect(screen.queryByRole('button', { name: /approve/i })).toBeNull()
    rerender(<ApprovalActions approvals={[{ ...approvals[0], status: 'APPROVED' }]} runState="PAUSED" onResolve={onResolve} />)
    expect(screen.queryByRole('button', { name: /approve/i })).toBeNull()
  })
})

describe('FR-026 automated accessibility scan', () => {
  it('reports no critical or serious violations across all seven named surfaces', async () => {
    const item = run('RUNNING', 'scan-run')
    const report: Report = { run: item, stages: [], gates: [], routing_decisions: [], artifacts: [], approvals: [] }
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/runs')) return json([item])
      if (url.endsWith('/api/v1/profiles')) return json([profile])
      if (url.endsWith('/api/v1/runs/scan-run')) return json(report)
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    const runButton = await screen.findByRole('button', { name: /Review a bounded workflow.*RUNNING/ })
    fireEvent.click(runButton)
    const detail = await screen.findByLabelText('Run detail surface')

    const containers = [
      render(<RunList runs={[run('RUNNING')]} selected="" onSelect={() => undefined} />).container,
      detail,
      render(<section aria-label="Route preview surface"><h2>Route preview</h2><RoutePreviewPanel state="ready" preview={preview} blocker={null} error="" onRemediate={() => undefined} /></section>).container,
      render(<ProfileReadinessPanel profiles={[profile]} disconnected={false} error="" />).container,
      render(<SharedStateSurface state="empty" message="No reports yet" />).container,
      render(<SharedStateSurface state="error" message="The report is unavailable" />).container,
      render(<SharedStateSurface state="disconnected" message="The local API is unavailable" />).container,
    ]
    const names = ['run-list', 'run-detail', 'route-preview', 'profile-readiness', 'shared-empty', 'shared-error', 'shared-disconnected']
    const failures: string[] = []
    for (let index = 0; index < containers.length; index++) {
      const result = await axe.run(containers[index], { rules: { 'color-contrast': { enabled: false } } })
      const serious = result.violations.filter((violation) => violation.impact === 'critical' || violation.impact === 'serious')
      if (serious.length) failures.push(`${names[index]}: ${serious.map((item) => item.id).join(', ')}`)
    }
    expect(failures).toEqual([])
  })
})

describe('FR-026 contrast and keyboard rules', () => {
  it('keeps primary muted text at 4.5:1 and defines visible focus and reduced motion', async () => {
    const css = readFileSync('src/style.css', 'utf8')
    const read = (token: string) => {
      const matches = [...css.matchAll(new RegExp(`${token}:\s*(#[0-9a-fA-F]{6})`, 'g'))]
      if (!matches.length) throw new Error(`missing CSS token ${token}`)
      return matches[matches.length - 1][1]
    }
    for (const color of [read('--ink'), read('--muted'), read('--muted-2')]) {
      expect(contrast(color, read('--panel'))).toBeGreaterThanOrEqual(4.5)
      expect(contrast(color, read('--bg'))).toBeGreaterThanOrEqual(4.5)
    }
    expect(css).toContain(':focus-visible')
    expect(css).toContain('prefers-reduced-motion:reduce')
  })
})

function json(value: unknown, status = 200) {
  return Promise.resolve({ ok: status >= 200 && status < 300, status, json: async () => value, text: async () => JSON.stringify(value) } as Response)
}

function contrast(foreground: string, background: string) {
  const luminance = (hex: string) => {
    const channels = hex.slice(1).match(/.{2}/g)!.map((channel) => parseInt(channel, 16) / 255)
    const [red, green, blue] = channels.map((channel) => channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4)
    return 0.2126 * red + 0.7152 * green + 0.0722 * blue
  }
  const one = luminance(foreground)
  const two = luminance(background)
  return (Math.max(one, two) + 0.05) / (Math.min(one, two) + 0.05)
}
