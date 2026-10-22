import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  runTone,
  summarizeRuns,
  type Profile,
  type Report,
  type Run,
} from './domain'

type View = 'workflow' | 'history' | 'profiles' | 'routing' | 'readiness'
type ConnectionState = 'loading' | 'connected' | 'disconnected'
type PreviewState = 'idle' | 'ready' | 'blocked' | 'disconnected'
type LocalEvent = { id?: string; run_id: string; seq: number; timestamp?: string; type: string; stage?: string; payload?: Record<string, unknown> }
type RoutePreview = { dry_run: boolean; execution: string; mode: string; decisions: Array<{ stage: string; route: { provider: string; profile: string; model?: string; effort?: string }; explanation: string }> }

export function App() {
  const [token, setToken] = useState('')
  const [runs, setRuns] = useState<Run[]>([])
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [report, setReport] = useState<Report | null>(null)
  const [events, setEvents] = useState<LocalEvent[]>([])
  const [selected, setSelected] = useState('')
  const [view, setView] = useState<View>('workflow')
  const [project, setProject] = useState('./examples/fake-provider-demo')
  const [prompt, setPrompt] = useState('Validate the complete SpecTide workflow')
  const [busy, setBusy] = useState(false)
  const [message, setMessage] = useState('Loading local run history…')
  const [connection, setConnection] = useState<ConnectionState>('loading')
  const [profileError, setProfileError] = useState('')
  const [preview, setPreview] = useState<RoutePreview | null>(null)
  const [previewState, setPreviewState] = useState<PreviewState>('idle')
  const [previewError, setPreviewError] = useState('')

  const api = useCallback(async <T,>(path: string, options: RequestInit = {}): Promise<T> => {
    const response = await fetch(path, {
      ...options,
      headers: {
        ...(options.headers ?? {}),
        ...(options.method && options.method !== 'GET' ? { 'X-SpecTide-CSRF': token } : {}),
      },
    })
    if (!response.ok) throw new Error((await response.text()) || `Request failed (${response.status})`)
    return response.json() as Promise<T>
  }, [token])

  const refresh = useCallback(async () => {
    try {
      const currentRuns = await api<Run[]>('/api/v1/runs')
      setRuns(currentRuns)
      const summary = summarizeRuns(currentRuns)
      setMessage(`${summary.active} active · ${summary.completed} completed · ${summary.total} durable runs`)
      if (selected) {
        const next = await api<Report>(`/api/v1/runs/${encodeURIComponent(selected)}`)
        setReport(next)
      }
    } catch (error) {
      setMessage(`Unable to refresh local state: ${String(error)}`)
      setConnection('disconnected')
    }
  }, [api, selected])

  useEffect(() => {
    let alive = true
    fetch('/api/v1/version')
      .then((response) => response.json())
      .then((version) => {
        if (alive) {
          setToken(version.csrf_token)
          setConnection('connected')
        }
      })
      .catch((error) => {
        if (alive) {
          setConnection('disconnected')
          setMessage(`Unable to connect to the local API: ${String(error)}`)
        }
      })
    return () => { alive = false }
  }, [])

  useEffect(() => {
    if (!token) return
    void refresh()
    void api<Profile[]>('/api/v1/profiles').then((items) => { setProfiles(items); setProfileError('') }).catch((error) => setProfileError(String(error)))
    const timer = window.setInterval(() => void refresh(), 1800)
    return () => window.clearInterval(timer)
  }, [api, refresh, token])

  useEffect(() => {
    setReport(null)
    setEvents([])
    if (!selected) return
    let live = true
    api<Report>(`/api/v1/runs/${encodeURIComponent(selected)}`)
      .then((next) => { if (live) setReport(next) })
      .catch((error) => setMessage(`Unable to load run: ${String(error)}`))
    const stream = new EventSource(`/api/v1/runs/${encodeURIComponent(selected)}/events?after=0`)
    const receive = (event: Event) => {
      const messageEvent = event as MessageEvent<string>
      try {
        const item = JSON.parse(messageEvent.data) as LocalEvent
        item.id = `${item.run_id}-${item.seq}`
        setEvents((current) => {
          if (current.some((known) => known.id === item.id)) return current
          return [item, ...current].slice(0, 80)
        })
      } catch { /* Ignore malformed frames and retain previously observed state. */ }
    }
    stream.onmessage = receive
    const types = [
      'run.created', 'run.RUNNING', 'run.PAUSED', 'run.COMPLETED', 'run.CANCELLED', 'run.FAILED', 'run.BLOCKED',
      'stage.started', 'stage.completed', 'stage.cancelled', 'analysis.clean', 'tests.passed', 'converge.converged',
      'routing.resolved', 'run.interrupted',
    ]
    types.forEach((type) => stream.addEventListener(type, receive))
    return () => {
      live = false
      stream.close()
    }
  }, [api, selected])

  const selectedRun = useMemo(() => runs.find((run) => run.id === selected), [runs, selected])
  const simulatedRunIDs = useMemo(() => {
    const decisions = report?.routing_decisions ?? []
    return report && decisions.length > 0 && decisions.every((decision) => decision.provider === 'fake') ? new Set([report.run.id]) : new Set<string>()
  }, [report])

  async function startRun(nextPrompt = prompt, nextProject = project) {
    if (!token || busy) return
    setBusy(true)
    setMessage('Starting a simulated local run…')
    try {
      const created = await api<Run>('/api/v1/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ project: nextProject, prompt: nextPrompt }),
      })
      setSelected(created.id)
      setView('workflow')
      await refresh()
      setMessage(`SIMULATED run ${created.id.slice(0, 8)} started`)
    } catch (error) {
      setMessage(`Unable to start run: ${String(error)}`)
    } finally {
      setBusy(false)
    }
  }

  async function cancelRun() {
    if (!selected) return
    try {
      await api(`/api/v1/runs/${encodeURIComponent(selected)}/cancel`, { method: 'POST' })
      setMessage('Cancellation requested')
      await refresh()
    } catch (error) {
      setMessage(`Unable to cancel run: ${String(error)}`)
    }
  }

  async function runControl(action: 'pause' | 'resume') {
    if (!selected) return
    try {
      await api(`/api/v1/runs/${encodeURIComponent(selected)}/${action}`, { method: 'POST' })
      setMessage(`${action === 'pause' ? 'Pause' : 'Resume'} requested`)
      await refresh()
    } catch (error) {
      setMessage(`Unable to ${action} run: ${String(error)}`)
    }
  }

  async function previewRoutes() {
    if (!token) {
      setPreviewState('disconnected')
      setPreviewError('Connect to the local API before previewing routes.')
      return
    }
    setPreviewError('')
    try {
      const result = await api<RoutePreview>('/api/v1/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ project, prompt, dry_run: true }),
      })
      setPreview(result)
      setPreviewState('ready')
    } catch (error) {
      const detail = String(error)
      setPreview(null)
      setPreviewError(detail)
      setPreviewState(connection === 'disconnected' ? 'disconnected' : 'blocked')
    }
  }

  return (
    <div className="layout">
      <aside className="rail">
        <div className="brand"><span className="brand-mark">S</span><span>Spec<span className="accent">Tide</span></span></div>
        <div className="locality"><span className="online-dot" />LOCAL CONTROL PLANE</div>
        <nav className="navigation" aria-label="Main navigation">
          <button className={view === 'workflow' ? 'nav-item active' : 'nav-item'} aria-current={view === 'workflow' ? 'page' : undefined} onClick={() => setView('workflow')}><span aria-hidden="true">⌁</span> Workflow control</button>
          <button className={view === 'history' ? 'nav-item active' : 'nav-item'} aria-current={view === 'history' ? 'page' : undefined} onClick={() => setView('history')}><span aria-hidden="true">◷</span> Run history</button>
          <button className={view === 'routing' ? 'nav-item active' : 'nav-item'} aria-current={view === 'routing' ? 'page' : undefined} onClick={() => setView('routing')}><span aria-hidden="true">⌖</span> Route preview</button>
          <button className={view === 'profiles' ? 'nav-item active' : 'nav-item'} aria-current={view === 'profiles' ? 'page' : undefined} onClick={() => setView('profiles')}><span aria-hidden="true">⌘</span> Profiles &amp; models</button>
          <button className={view === 'readiness' ? 'nav-item active' : 'nav-item'} aria-current={view === 'readiness' ? 'page' : undefined} onClick={() => setView('readiness')}><span aria-hidden="true">✓</span> Profile readiness</button>
        </nav>
        <div className="rail-footer"><span className="sim-pill">SIMULATED</span><p>Provider stages are offline fixtures. Live provider execution remains disabled.</p><code>127.0.0.1</code></div>
      </aside>

      <main className="main">
        <header className="page-head">
          <div><div className="eyebrow">SPEC-KIT · LOCAL WORKSPACE</div><h1>{view === 'profiles' ? 'Profiles & models' : view === 'readiness' ? 'Profile readiness' : view === 'routing' ? 'Route preview' : view === 'history' ? 'Run history' : 'Workflow control'}</h1><p className="status-line" role="status"><span className={`status-dot ${connection === 'disconnected' ? 'offline' : ''}`} />{message}</p></div>
          <button className="button button-primary" disabled={busy || !token} onClick={() => void startRun('Run deterministic offline verification', './examples/fake-provider-demo')}><span className="button-symbol">＋</span> Run offline demo</button>
        </header>

        {connection === 'disconnected' && <SharedStateSurface state="disconnected" message="The local SpecTide API cannot be reached. Persisted dashboard data may be stale." />}
        {view === 'routing' ? (
          <section className="panel route-preview-panel" aria-label="Route preview surface"><div className="panel-head"><div><div className="eyebrow">DRY RUN · NO WORKFLOW STARTED</div><h2>Resolved provider routes</h2></div><span className="count">NOT RUN</span></div><p className="surface-copy">Preview the configured provider for each stage before starting work. The preview does not create a run or send repository content to a provider.</p><button className="button button-secondary preview-button" onClick={() => void previewRoutes()} disabled={!token}>Preview routes<span aria-hidden="true">→</span></button><RoutePreviewPanel state={previewState} preview={preview} error={previewError} /><div className="route-inputs"><label>Project path<input value={project} onChange={(event) => setProject(event.target.value)} /></label><label>Task prompt<textarea rows={2} value={prompt} onChange={(event) => setPrompt(event.target.value)} /></label></div></section>
        ) : view === 'readiness' ? (
          <ProfileReadinessPanel profiles={profiles} disconnected={connection === 'disconnected'} error={profileError} />
        ) : view === 'profiles' ? (
          <section className="panel profile-panel">
            <div className="panel-head"><div><div className="eyebrow">LOCAL CONFIGURATION</div><h2>Registered profiles</h2></div><span className="count">{profiles.length} profiles</span></div>
            {profileError ? <div className="state-banner error" role="alert"><strong>Error loading profiles</strong><span>{profileError}</span></div> : profiles.length ? <div className="profile-list">{profiles.map((profile) => <article className="profile-row" key={profile.id}><div className="provider-icon">{profile.provider.slice(0, 1).toUpperCase()}</div><div className="profile-name"><strong>{profile.name}</strong><span>{profile.provider} · {profile.executable}</span></div><span className="scope">{profile.allowed_scopes.join(', ')}</span><span className="status-tag">{profile.capability?.authenticated ?? 'NOT PROBED'}</span></article>)}</div> : <div className="empty-state" role="status"><span className="empty-icon" aria-hidden="true">⌘</span><strong>No profiles registered</strong><p>Add a local profile with <code>spectide profiles add</code>. Credentials stay in the provider's own account store.</p></div>}
            <div className="notice"><strong>Capability boundary</strong><span>Profile presence does not verify authentication. Use <code>spectide doctor</code> to check local readiness.</span></div>
          </section>
        ) : view === 'history' ? (
          <section className="panel history-panel"><div className="panel-head"><div><div className="eyebrow">PERSISTED RUNS</div><h2>All runs</h2></div><span className="count">{runs.length} total</span></div><RunList runs={runs} selected={selected} onSelect={setSelected} simulatedRunIDs={simulatedRunIDs} disconnected={connection === 'disconnected'} /></section>
        ) : (
          <>
            <section className="overview-grid">
              <div className="panel runs-panel"><div className="panel-head"><div><div className="eyebrow">WORKSPACE ACTIVITY</div><h2>Recent runs</h2></div><button className="text-button" onClick={() => setView('history')}>View history <span>↗</span></button></div><RunList runs={runs.slice(0, 6)} selected={selected} onSelect={setSelected} simulatedRunIDs={simulatedRunIDs} disconnected={connection === 'disconnected'} /></div>
              <div className="panel start-panel"><div className="panel-head"><div><div className="eyebrow">OFFLINE PATH</div><h2>Start a simulated run</h2></div><span className="small-icon">↗</span></div>
                <form onSubmit={(event) => { event.preventDefault(); void startRun() }}>
                  <label>Project path<input required value={project} onChange={(event) => setProject(event.target.value)} /></label>
                  <label>Task prompt<textarea required rows={3} value={prompt} onChange={(event) => setPrompt(event.target.value)} /></label>
                  <button className="button button-secondary" disabled={busy || !token}>{busy ? 'Starting…' : 'Start simulated workflow'}<span>→</span></button>
                </form>
                <div className="inline-note"><span className="sim-dot" />Fixture stages · local SQLite evidence · no provider account used</div>
              </div>
            </section>

            <section className="panel detail-panel" aria-label="Run detail surface" aria-live="polite">
              <div className="panel-head"><div><div className="eyebrow">OBSERVED WORKFLOW</div><h2>{selectedRun ? <>Run <code>{selectedRun.id.slice(0, 8)}</code></> : 'Run detail'}</h2></div>{selectedRun && <RunDetailStatus run={selectedRun} onPause={() => void runControl('pause')} onResume={() => void runControl('resume')} onCancel={() => void cancelRun()} />}</div>
              {!report ? <div className="empty-inline">Select a run to inspect its stages, routes, and evidence.</div> : <>
                <div className="run-context"><span className="context-label">TASK</span><span>{report.run.prompt}</span><span className="context-label">PROJECT</span><code>{report.run.project}</code></div>
                <div className="stage-list">{report.stages.map((stage, index) => <div className="stage-row" key={stage.id}><span className={stage.state === 'SUCCEEDED' ? 'stage-index complete' : 'stage-index'} aria-hidden="true">{stage.state === 'SUCCEEDED' ? '✓' : String(index + 1).padStart(2, '0')}</span><strong>{stage.key.replaceAll('_', ' ')}</strong><span className="stage-route">{stage.provider} <i>/</i> {stage.profile} <i>·</i> {stage.model || 'default'} <i>·</i> attempt {stage.attempt}{stage.provider === 'fake' && <b className="simulated-state">SIMULATED</b>}</span><span className={`stage-state ${stage.state === 'SUCCEEDED' ? 'good' : ''}`}>{stage.state}</span></div>)}</div>
                {!!report.routing_decisions?.length && <details className="route-details"><summary>Routing decisions <span>{report.routing_decisions.length} stages</span></summary><div className="route-list">{report.routing_decisions.map((decision) => <div key={decision.stage}><strong>{decision.stage}</strong><span>{decision.provider} / {decision.profile}</span><small>{decision.explanation}</small></div>)}</div></details>}
              </>}
            </section>

            <section className="evidence-grid">
              <div className="panel evidence-panel"><div className="panel-head"><div><div className="eyebrow">DURABLE EVENTS</div><h2>Activity stream</h2></div><span className="live-label"><i /> LIVE</span></div><EventList events={events} /></div>
              <div className="panel evidence-panel"><div className="panel-head"><div><div className="eyebrow">RECORDED VERDICTS</div><h2>Quality gates</h2></div><span className="count">{report?.gates.length ?? 0}</span></div><GateList gates={report?.gates ?? []} /></div>
            </section>
            {report && <section className="panel artifacts-panel"><div className="panel-head"><div><div className="eyebrow">RUN OUTPUTS</div><h2>Artifacts &amp; approvals</h2></div></div>{report.artifacts?.length || report.approvals?.length ? <div className="artifact-list">{report.artifacts?.map((artifact) => <div className="artifact-row" key={artifact.id}><span>▤</span><strong>{artifact.path}</strong><code>{artifact.sha256.slice(0, 12)}</code></div>)}{report.approvals?.map((approval) => <div className="artifact-row" key={approval.id}><span>◇</span><strong>{approval.action}</strong><span className="state-pill quiet">{approval.status}</span></div>)}</div> : <div className="empty-inline">No artifacts or approvals recorded for this run.</div>}</section>}
          </>
        )}
        <footer className="footer"><span>SpecTide <b>0.1.0-dev</b></span><span>State is read from local SQLite · provider fixtures are marked simulated</span></footer>
      </main>
    </div>
  )
}

export function RunList({ runs, selected, onSelect, simulatedRunIDs = new Set<string>(), disconnected = false }: { runs: Run[]; selected: string; onSelect: (id: string) => void; simulatedRunIDs?: Set<string>; disconnected?: boolean }) {
  if (!runs.length) return <><div className="empty-state compact" role="status" aria-label="Empty run list"><span className="empty-icon" aria-hidden="true">⌁</span><strong>No runs recorded yet</strong><p>Start the offline demo to see the deterministic lifecycle.</p></div>{disconnected && <SharedStateSurface state="disconnected" message="Run history may be stale while the local API is unavailable." />}</>
  return <>{disconnected && <SharedStateSurface state="disconnected" message="Run history may be stale while the local API is unavailable." />}<div className="run-list" aria-label="Run list">{runs.map((run) => <button key={run.id} className={`run-card ${selected === run.id ? 'selected' : ''}`} onClick={() => onSelect(run.id)}><span className="run-avatar" aria-hidden="true">{run.state === 'COMPLETED' ? '✓' : run.state === 'RUNNING' ? '◌' : '·'}</span><span className="run-copy"><strong>{run.prompt}</strong><span>{run.project} <i aria-hidden="true">·</i> {new Date(run.created_at).toLocaleString()}</span></span>{simulatedRunIDs.has(run.id) && <span className="execution-tag">SIMULATED</span>}<span className={`state-pill ${runTone(run.state)}`}>{run.state}</span></button>)}</div></>
}

export function RunDetailStatus({ run, onPause, onResume, onCancel }: { run: Run; onPause: () => void; onResume: () => void; onCancel: () => void }) {
  return <div className="detail-actions"><span className={`state-pill ${runTone(run.state)}`}>{run.state}</span>{run.state === 'RUNNING' && <button className="button button-secondary control-button" onClick={onPause}>Pause</button>}{run.state === 'PAUSED' && <button className="button button-secondary control-button" onClick={onResume}>Resume</button>}{['RUNNING', 'PAUSED'].includes(run.state) && <button className="button button-danger" onClick={onCancel}>Cancel run</button>}</div>
}

function EventList({ events }: { events: LocalEvent[] }) {
  if (!events.length) return <div className="empty-inline">Select a run to follow its persisted event stream.</div>
  return <div className="event-list">{events.map((event, index) => <div className="event-row" key={`${event.id}-${index}`}><time>{event.timestamp ? new Date(event.timestamp).toLocaleTimeString() : '—'}</time><span className="event-dot" /><span className="event-copy"><strong>{event.type.replaceAll('.', ' ')}</strong>{event.stage && <small>{event.stage}</small>}</span></div>)}</div>
}

function GateList({ gates }: { gates: Report['gates'] }) {
  if (!gates.length) return <div className="empty-inline">Quality evidence has not been recorded.</div>
  return <div className="gate-list">{gates.map((gate) => <div className="gate-row" key={gate.key}><span className="gate-symbol">{gate.verdict === 'PASS' ? '✓' : '!'}</span><span className="gate-name"><strong>{gate.key.replaceAll('_', ' ')}</strong><small>{gate.evidence}</small></span><span className={`gate-verdict ${gate.verdict === 'PASS' ? 'good' : ''}`}>{gate.verdict}</span></div>)}</div>
}

export function RoutePreviewPanel({ state, preview, error }: { state: PreviewState; preview: RoutePreview | null; error: string }) {
  if (state === 'idle') return <div className="empty-inline" role="status">Route preview has not been run. No work starts from this screen.</div>
  if (state === 'disconnected') return <SharedStateSurface state="disconnected" message={error || 'Connect to the local API to preview routes.'} />
  if (state === 'blocked') return <SharedStateSurface state="error" title="Blocked" message={error || 'The configured route is not authorized.'} />
  if (!preview?.decisions.length) return <div className="empty-inline" role="status">No route decisions were returned.</div>
  return <div className="route-preview-results" role="status" aria-label="Ready route preview"><p><strong>Ready · NOT RUN</strong><span>Mode: {preview.mode} · Execution: {preview.execution}</span></p><div className="route-list">{preview.decisions.map((decision) => <div key={decision.stage}><strong>{decision.stage.replaceAll('_', ' ')}</strong><span>{decision.route.provider} / {decision.route.profile}</span><small>{decision.explanation}</small></div>)}</div></div>
}

export function SharedStateSurface({ state, title, message }: { state: 'empty' | 'error' | 'disconnected'; title?: string; message: string }) {
  if (state === 'empty') return <div className="empty-inline" role="status" aria-label="Empty state">{message}</div>
  return <div className={`state-banner ${state === 'error' ? 'error' : 'disconnected'}`} role={state === 'error' ? 'alert' : 'status'} aria-label={`${state} state`}><strong>{title ?? (state === 'error' ? 'Error' : 'Disconnected')}</strong><span>{message}</span></div>
}

export function ProfileReadinessPanel({ profiles, disconnected, error }: { profiles: Profile[]; disconnected: boolean; error: string }) {
  return <section className="panel readiness-panel" aria-label="Profile readiness surface"><div className="panel-head"><div><div className="eyebrow">LOCAL PROVIDER CAPABILITIES</div><h2>Profile readiness</h2></div><span className="count">{profiles.length} profiles</span></div>
    {disconnected ? <SharedStateSurface state="disconnected" message="Profile readiness cannot be refreshed while the local API is unavailable." /> : error ? <SharedStateSurface state="error" message={error} /> : profiles.length === 0 ? <div className="empty-state" role="status"><strong>No profiles to assess</strong><p>Register a provider profile to see its observed readiness.</p></div> : <div className="readiness-list">{profiles.map((profile) => {
      const capability = profile.capability
      const ready = capability?.authenticated === 'AUTHENTICATED' && capability.executable_found && capability.read_only_execution && capability.workspace_write
      const status = !capability ? 'NOT PROBED' : ready ? 'READY' : capability.authenticated === 'UNAUTHENTICATED' ? 'DISCONNECTED' : 'NOT READY'
      return <article className="readiness-row" key={profile.id}><div><strong>{profile.name}</strong><span>{profile.provider} · {profile.executable}</span></div><span className={`readiness-status ${status === 'READY' ? 'good' : status === 'DISCONNECTED' ? 'bad' : ''}`}>{status}</span><small>{capability?.version ? `Version ${capability.version}` : 'Version and authentication are not verified'}</small></article>
    })}</div>}
    <div className="notice"><strong>Credentials stay local</strong><span>Readiness uses persisted non-secret capability evidence only.</span></div>
  </section>
}
