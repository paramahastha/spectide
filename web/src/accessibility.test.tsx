// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { readFileSync } from 'node:fs'
import axe from 'axe-core'
import { App, ProfileReadinessPanel, RoutePreviewPanel, RunDetailStatus, RunList, SharedStateSurface } from './App'
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
  decisions: [{ stage: 'implement', route: { provider: 'fake', profile: 'offline' }, explanation: 'deterministic offline route' }],
}

describe('FR-026 dashboard page and state matrix', () => {
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
    const { rerender } = render(<RoutePreviewPanel state="ready" preview={preview} error="" />)
    expect(screen.getByText(/Ready · NOT RUN/)).toBeTruthy()
    rerender(<RoutePreviewPanel state="blocked" preview={null} error="PROFILE_NOT_ALLOWED" />)
    expect(screen.getByRole('alert').textContent).toContain('PROFILE_NOT_ALLOWED')
    rerender(<RoutePreviewPanel state="disconnected" preview={null} error="API unavailable" />)
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
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') && init?.method === 'POST') {
        requestBody = JSON.parse(String(init.body)) as Record<string, unknown>
        return json(preview)
      }
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    Object.defineProperty(globalThis.crypto, 'randomUUID', { configurable: true, value: () => 'preview-key-12345678' })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: /Route preview/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Preview routes/ }))
    expect(await screen.findByText(/Ready · NOT RUN/)).toBeTruthy()
    expect(requestBody?.dry_run).toBe(true)
    expect(requestBody?.project).toBe('./examples/fake-provider-demo')
  })

  it.each([
    ['empty', 'No records yet'],
    ['error', 'The stored report could not be loaded'],
    ['disconnected', 'The local API cannot be reached'],
  ] as const)('renders the shared %s state', (state, message) => {
    render(<SharedStateSurface state={state} message={message} />)
    expect(screen.getByText(message)).toBeTruthy()
  })

  it('opens a persisted run detail and exposes stage, gate, and run states', async () => {
    const item = run('BLOCKED', 'run-1')
    const report: Report = {
      run: item,
      stages: [{ id: 'stage-1', key: 'analyze', state: 'SKIPPED', provider: 'fake', profile: 'offline', model: '', effort: '', attempt: 1 }],
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
    expect(screen.getAllByText('BLOCKED').length).toBeGreaterThanOrEqual(1)
    expect(screen.getAllByText('SIMULATED').length).toBeGreaterThanOrEqual(2)
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
      render(<section aria-label="Route preview surface"><h2>Route preview</h2><RoutePreviewPanel state="ready" preview={preview} error="" /></section>).container,
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
