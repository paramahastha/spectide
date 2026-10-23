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
    const { rerender } = render(<RoutePreviewPanel state="ready" preview={preview} error="" />)
    expect(screen.getByText(/Ready · NOT RUN/)).toBeTruthy()
    expect(screen.getByText(/fixture-model.*high/)).toBeTruthy()
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
      if (url.endsWith('/api/v1/profiles')) return json([])
      if (url.endsWith('/api/v1/runs') && init?.method === 'POST') return json(preview)
      if (url.endsWith('/api/v1/runs')) return json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    Object.defineProperty(globalThis.crypto, 'randomUUID', { configurable: true, value: () => 'preview-key-12345678' })
    render(<App />)
    fireEvent.click(await screen.findByRole('button', { name: /Route preview/ }))
    fireEvent.click(await screen.findByRole('button', { name: /Preview routes/ }))
    expect(await screen.findByText(/fixture-model.*high/)).toBeTruthy()
    fireEvent.change(screen.getByLabelText('Project path'), { target: { value: '/workspace/other' } })
    expect(screen.getByText(/Route preview has not been run/)).toBeTruthy()
    expect(screen.queryByText(/fixture-model/)).toBeNull()
  })

  it('clears disconnected state after run-list recovery without a selected event stream', async () => {
    vi.useFakeTimers()
    let disconnected = true
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url.endsWith('/api/v1/version')) return json({ csrf_token: 'csrf-fixture' })
      if (url.endsWith('/api/v1/profiles')) return disconnected ? Promise.reject(new Error('offline')) : json([])
      if (url.endsWith('/api/v1/runs')) return disconnected ? Promise.reject(new Error('offline')) : json([])
      return json({}, 404)
    }))
    vi.stubGlobal('EventSource', class { close() {} addEventListener() {} })
    render(<App />)
    await act(async () => { await vi.advanceTimersByTimeAsync(0) })
    expect(screen.getAllByLabelText('disconnected state').length).toBeGreaterThan(0)
    disconnected = false
    await act(async () => { await vi.advanceTimersByTimeAsync(1800) })
    expect(screen.queryByLabelText('disconnected state')).toBeNull()
    vi.useRealTimers()
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
    expect(screen.getAllByText(/RECONNECTING/).length).toBe(2)

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
      expect(container.querySelector('[aria-label="Event stream connected"]')?.textContent).toContain('LIVE')
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
