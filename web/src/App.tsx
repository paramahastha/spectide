import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  runTone,
  summarizeRuns,
  type Profile,
  type Approval,
  type ExecutionAttempt,
  type QualityExecution,
  type Report,
  type Run,
  type Project,
  type WorkspaceStatus,
} from './domain'

type View = 'workflow' | 'history' | 'profiles' | 'routing' | 'readiness'
type ConnectionState = 'loading' | 'connected' | 'disconnected'
type PreviewState = 'idle' | 'ready' | 'blocked' | 'disconnected'
type LocalEvent = { id?: string; run_id: string; seq: number; timestamp?: string; type: string; stage?: string; payload?: Record<string, unknown> }
export type RoutePreview = { dry_run: boolean; execution: string; mode: string; decisions: Array<{ stage: string; route: { provider: string; profile: string; model?: string; effort?: string }; explanation: string }> }

function isCsrfRequired(body: string): boolean {
  try {
    const response = JSON.parse(body) as { error?: { code?: string } }
    return response.error?.code === 'CSRF_REQUIRED'
  } catch {
    return false
  }
}

export function App() {
  const [token, setToken] = useState('')
  const tokenRef = useRef('')
  const handshakeInFlight = useRef<Promise<string> | null>(null)
  const [appVersion, setAppVersion] = useState('0.1.0-dev')
  const [runs, setRuns] = useState<Run[]>([])
  const [totalRunCount, setTotalRunCount] = useState<number | null>(null)
  const [projects, setProjects] = useState<Project[]>([])
  const [workspaceStatus, setWorkspaceStatus] = useState<WorkspaceStatus | null>(null)
  const [activeProjectId, setActiveProjectId] = useState('')
  const [newProjectPath, setNewProjectPath] = useState('')
  const [addProjectError, setAddProjectError] = useState('')
  const [addingProject, setAddingProject] = useState(false)
  const [profiles, setProfiles] = useState<Profile[]>([])
  const [report, setReport] = useState<Report | null>(null)
  const [events, setEvents] = useState<LocalEvent[]>([])
  const lastEventSeq = useRef(0)
  const [selected, setSelected] = useState('')
  const selectedRef = useRef('')
  selectedRef.current = selected
  const refreshRequest = useRef(0)
  const previewRequest = useRef(0)
  const [view, setView] = useState<View>('workflow')
  const [project, setProject] = useState('./examples/fake-provider-demo')
  const [prompt, setPrompt] = useState('Validate the complete SpecTide workflow')
  const [busy, setBusy] = useState(false)
  const [approvalBusy, setApprovalBusy] = useState('')
  const [message, setMessage] = useState('Loading local run history…')
  const [apiConnection, setApiConnection] = useState<ConnectionState>('loading')
  const [profileConnection, setProfileConnection] = useState<ConnectionState>('loading')
  const [streamConnection, setStreamConnection] = useState<ConnectionState>('loading')
  const [profileError, setProfileError] = useState('')
  const [preview, setPreview] = useState<RoutePreview | null>(null)
  const [previewState, setPreviewState] = useState<PreviewState>('idle')
  const [previewError, setPreviewError] = useState('')
  const connection: ConnectionState = apiConnection === 'disconnected' || profileConnection === 'disconnected' || (selected !== '' && streamConnection === 'disconnected')
    ? 'disconnected'
    : apiConnection === 'loading' || profileConnection === 'loading' || (selected !== '' && streamConnection === 'loading')
      ? 'loading'
      : 'connected'

  const refreshHandshake = useCallback(async (): Promise<string> => {
    if (handshakeInFlight.current) return handshakeInFlight.current
    const request = (async () => {
      const response = await fetch('/api/v1/version')
      if (!response.ok) throw new Error(`Request failed (${response.status})`)
      const version = await response.json() as { csrf_token?: string; version?: string }
      if (!version.csrf_token) throw new Error('Version response did not include a CSRF token')
      tokenRef.current = version.csrf_token
      setToken(version.csrf_token)
      if (version.version) setAppVersion(version.version)
      setApiConnection('connected')
      setMessage('Connected to the local API')
      return version.csrf_token
    })()
    handshakeInFlight.current = request
    try {
      return await request
    } finally {
      if (handshakeInFlight.current === request) handshakeInFlight.current = null
    }
  }, [])

  const api = useCallback(async <T,>(path: string, options: RequestInit = {}): Promise<T> => {
    const response = await fetch(path, {
      ...options,
      headers: {
        ...(options.headers ?? {}),
        ...(options.method && options.method !== 'GET' ? { 'X-SpecTide-CSRF': tokenRef.current } : {}),
      },
    })
    if (!response.ok) {
      const body = await response.text()
      if (response.status === 403 && isCsrfRequired(body)) {
        // Refresh the capability for the next user action, but never replay a
        // mutation whose server-side outcome may be ambiguous.
        try { await refreshHandshake() } catch { /* Preserve the original request failure. */ }
      }
      throw new Error(body || `Request failed (${response.status})`)
    }
    const totalCount = response.headers?.get?.('X-Total-Count')
    if (totalCount !== null && Number.isFinite(Number(totalCount))) setTotalRunCount(Number(totalCount))
    return response.json() as Promise<T>
  }, [refreshHandshake])

  const refresh = useCallback(async () => {
    const request = ++refreshRequest.current
    const requestedSelection = selectedRef.current
    try {
      const query = activeProjectId ? `?project_id=${encodeURIComponent(activeProjectId)}` : ''
      const currentRuns = await api<Run[]>(`/api/v1/runs${query}`)
      if (request !== refreshRequest.current) return
      setRuns(currentRuns)
      setApiConnection('connected')
      const summary = summarizeRuns(currentRuns)
      setMessage(`${summary.active} active · ${summary.completed} completed · ${totalRunCount ?? summary.total} durable runs`)
      if (requestedSelection) {
        const next = await api<Report>(`/api/v1/runs/${encodeURIComponent(requestedSelection)}`)
        if (request === refreshRequest.current && selectedRef.current === requestedSelection) setReport(next)
      }
    } catch (error) {
      if (request !== refreshRequest.current || selectedRef.current !== requestedSelection) return
      setMessage(`Unable to refresh local state: ${String(error)}`)
      setApiConnection('disconnected')
    }
  }, [activeProjectId, api, totalRunCount])

  useEffect(() => {
    let alive = true
    let timer = 0
    const load = async () => {
      try {
        await refreshHandshake()
      } catch (error) {
        if (alive) {
          setApiConnection('disconnected')
          setMessage(`Unable to connect to the local API: ${String(error)}`)
          timer = window.setTimeout(() => { if (alive) void load() }, 1800)
        }
      }
    }
    void load()
    return () => { alive = false; window.clearTimeout(timer) }
  }, [refreshHandshake])

  useEffect(() => {
    if (!token) return
    void refresh()
    const loadProjects = () => void api<Project[]>('/api/v1/projects').then((items) => {
      const available = Array.isArray(items) ? items : []
      setProjects(available)
      const saved = localStorage.getItem('spectide.project-selection') ?? ''
      const savedIsValid = saved !== '' && available.some((item) => item.id === saved)
      if (saved && !savedIsValid) localStorage.removeItem('spectide.project-selection')
      setActiveProjectId((current) => current && available.some((item) => item.id === current) ? current : savedIsValid ? saved : '')
    }).catch(() => setProjects([]))
    loadProjects()
    void api<WorkspaceStatus>('/api/v1/workspace/status').then(setWorkspaceStatus).catch(() => setWorkspaceStatus(null))
    const loadProfiles = () => void api<Profile[]>('/api/v1/profiles').then((items) => { setProfiles(Array.isArray(items) ? items : []); setProfileError(''); setProfileConnection('connected') }).catch((error) => { setProfileError(String(error)); setProfileConnection('disconnected') })
    loadProfiles()
    const timer = window.setInterval(() => void refresh(), 1800)
    const profileTimer = window.setInterval(loadProfiles, 1800)
    const projectTimer = window.setInterval(loadProjects, 5000)
    return () => { window.clearInterval(timer); window.clearInterval(profileTimer); window.clearInterval(projectTimer) }
  }, [api, refresh, token])

  useEffect(() => {
    if (activeProjectId && !projects.some((item) => item.id === activeProjectId)) {
      setActiveProjectId('')
      localStorage.removeItem('spectide.project-selection')
      setSelected('')
      setReport(null)
      setEvents([])
    } else if (activeProjectId) {
      localStorage.setItem('spectide.project-selection', activeProjectId)
      setSelected('')
      setReport(null)
      setEvents([])
    }
  }, [activeProjectId, projects])

  useEffect(() => {
    setReport(null)
    setEvents([])
    lastEventSeq.current = 0
    if (!selected) return
    let live = true
    setStreamConnection('loading')
    let refreshTimer = 0
    let handshakeRetryTimer = 0
    let reconnecting = false
    api<Report>(`/api/v1/runs/${encodeURIComponent(selected)}`)
      .then((next) => { if (live) setReport(next) })
      .catch((error) => setMessage(`Unable to load run: ${String(error)}`))
    let stream: EventSource
    let reconnectTimer = 0
    const receive = (event: Event) => {
      const messageEvent = event as MessageEvent<string>
      try {
        const item = JSON.parse(messageEvent.data) as LocalEvent
        if (!item.run_id || item.run_id !== selected || !Number.isSafeInteger(item.seq) || item.seq < 1) return
        item.id = `${item.run_id}-${item.seq}`
        lastEventSeq.current = Math.max(lastEventSeq.current, item.seq)
        setEvents((current) => {
          if (current.some((known) => known.id === item.id)) return current
          return [...current, item].sort((a, b) => a.seq - b.seq).slice(-80)
        })
        window.clearTimeout(refreshTimer)
        refreshTimer = window.setTimeout(() => { if (live) void refresh() }, 30)
      } catch { /* Ignore malformed frames and retain previously observed state. */ }
    }
    const types = [
      'run.created', 'run.RUNNING', 'run.PAUSED', 'run.COMPLETED', 'run.CANCELLED', 'run.FAILED', 'run.BLOCKED',
      'run.paused', 'run.resumed', 'run.interrupted', 'run.recovery_claimed', 'run.acceptance',
      'stage.created', 'stage.transition', 'stage.started', 'stage.completed', 'stage.cancelled', 'stage.failed', 'stage.blocked',
      'stage.owner_artifact_present', 'stage.owner_artifact_invalid', 'analysis.clean', 'analysis.failed', 'analysis.remediated',
      'tests.passed', 'tests.failed', 'converge.converged', 'convergence.report', 'routing.resolved',
      'approval.created', 'approval.resolved', 'approval.denied', 'quality.inventory_initialized', 'quality.gate_recorded', 'quality.execution_completed',
      'execution.attempt_started', 'execution.attempt_completed', 'execution.attempt_interrupted', 'stage.attempt_interrupted', 'checkpoint.failed', 'feature.directory_failed', 'workspace.lock_denied',
      'profile.capability_invalidated',
      'provider.event', 'provider.events.dropped', 'provider.started', 'provider.result', 'provider.failed', 'provider.completed',
      'provider.codex.thread.started', 'provider.codex.turn.started', 'provider.codex.turn.completed', 'provider.codex.turn.failed', 'provider.codex.item.started', 'provider.codex.item.updated', 'provider.codex.item.completed', 'provider.codex.rate_limits', 'provider.codex.rate_limits.error', 'provider.codex.error',
      'provider.claude.system', 'provider.claude.assistant', 'provider.claude.content_block_start', 'provider.claude.content_block_delta', 'provider.claude.content_block_stop', 'provider.claude.message_start', 'provider.claude.message_delta', 'provider.claude.message_stop', 'provider.claude.result', 'provider.claude.ping', 'provider.claude.error',
      'provider.opencode.step_start', 'provider.opencode.text', 'provider.opencode.reasoning', 'provider.opencode.tool_use', 'provider.opencode.tool_result', 'provider.opencode.step_finish', 'provider.opencode.session.idle', 'provider.opencode.step_error', 'provider.opencode.error', 'provider.fake.event', 'provider.fake.started', 'provider.fake.result',
    ]
    const connect = () => {
      if (!live) return
      stream = new EventSource(`/api/v1/runs/${encodeURIComponent(selected)}/events?after=${lastEventSeq.current}`)
      stream.onmessage = receive
      stream.onopen = () => {
        setStreamConnection('connected')
        window.clearTimeout(refreshTimer)
        refreshTimer = window.setTimeout(() => { if (live) void refresh() }, 0)
        if (reconnecting) {
          reconnecting = false
          const renewToken = () => {
            if (!live) return
            void refreshHandshake().catch(() => {
              handshakeRetryTimer = window.setTimeout(renewToken, 1800)
            })
          }
          renewToken()
        }
      }
      stream.onerror = () => {
        if (!live) return
        reconnecting = true
        setStreamConnection('disconnected')
        setMessage('Event stream disconnected · waiting to reconnect')
        stream.close()
        window.clearTimeout(reconnectTimer)
        reconnectTimer = window.setTimeout(connect, 500)
      }
      types.forEach((type) => stream.addEventListener(type, receive))
    }
    connect()
    return () => {
      live = false
      window.clearTimeout(refreshTimer)
      window.clearTimeout(reconnectTimer)
      window.clearTimeout(handshakeRetryTimer)
      stream.close()
    }
  }, [api, refresh, refreshHandshake, selected])

  useEffect(() => {
    const request = ++previewRequest.current
    setPreview(null)
    setPreviewState('idle')
    setPreviewError('')
    return () => { if (previewRequest.current === request) previewRequest.current++ }
  }, [project, prompt])

  useEffect(() => {
    if (view === 'routing' && connection === 'disconnected' && !token) {
      setPreviewState('disconnected')
      setPreviewError('Connect to the local API before previewing routes.')
    }
  }, [connection, token, view])

  const selectedRun = useMemo(() => runs.find((run) => run.id === selected), [runs, selected])
  const activeProject = projects.find((item) => item.id === activeProjectId)
  async function addProject(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!newProjectPath.trim() || addingProject) return
    setAddingProject(true); setAddProjectError('')
    try {
      const created = await api<Project>('/api/v1/projects', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ path: newProjectPath.trim() }) })
      setProjects((current) => current.some((item) => item.id === created.id) ? current : [...current, created])
      setActiveProjectId(created.id)
      setNewProjectPath('')
      setMessage(`${created.display_name} is ready to configure`)
    } catch (error) { setAddProjectError(String(error)) }
    finally { setAddingProject(false) }
  }
  async function startRun(nextPrompt = prompt) {
    if (!token || busy || !activeProjectId) { if (!activeProjectId) setMessage('Select a registered project before starting work'); return }
    setBusy(true)
    setMessage('Starting a workflow using the configured routes…')
    try {
      const created = await api<Run>('/api/v1/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ project_id: activeProjectId, prompt: nextPrompt }),
      })
      setSelected(created.id)
      setView('workflow')
      await refresh()
      setMessage(`Workflow run ${created.id.slice(0, 8)} started`)
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

  async function resolveApproval(approvalID: string, decision: 'approved' | 'denied') {
    if (!selected || approvalBusy) return
    setApprovalBusy(approvalID)
    try {
      await api(`/api/v1/runs/${encodeURIComponent(selected)}/approvals/${encodeURIComponent(approvalID)}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ decision }),
      })
      setMessage(decision === 'approved' ? 'Workspace write approved; workflow continuation requested' : 'Workspace write denied; workflow will stop before writing')
      await refresh()
    } catch (error) {
      setMessage(`Unable to resolve approval: ${String(error)}`)
    } finally {
      setApprovalBusy('')
    }
  }

  async function previewRoutes() {
    if (!token) {
      setPreviewState('disconnected')
      setPreviewError('Connect to the local API before previewing routes.')
      return
    }
    setPreviewError('')
    const request = ++previewRequest.current
    if (!activeProjectId) { setPreviewState('blocked'); setPreviewError('Select a registered project before previewing routes.'); return }
    const intent = `${activeProjectId}\u0000${prompt}`
    try {
      const result = await api<RoutePreview>('/api/v1/runs', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() },
        body: JSON.stringify({ project_id: activeProjectId, prompt, dry_run: true }),
      })
      if (request !== previewRequest.current || intent !== `${activeProjectId}\u0000${prompt}`) return
      setPreview(result)
      setPreviewState('ready')
    } catch (error) {
      if (request !== previewRequest.current || intent !== `${activeProjectId}\u0000${prompt}`) return
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
        <div className="rail-footer"><span className="sim-pill">LOCAL</span><p>Each run is labeled from its persisted route and execution evidence.</p><code>127.0.0.1</code></div>
      </aside>

      <main className="main">
        <header className="page-head">
          <div><div className="eyebrow">SPEC-KIT · LOCAL WORKSPACE</div><h1>{view === 'profiles' ? 'Profiles & models' : view === 'readiness' ? 'Profile readiness' : view === 'routing' ? 'Route preview' : view === 'history' ? 'Run history' : 'Workflow control'}</h1><p className="status-line" role="status"><span className={`status-dot ${connection === 'disconnected' ? 'offline' : ''}`} />{message}</p></div>
          <button className="button button-primary" disabled={busy || !token || !activeProjectId} onClick={() => void startRun('Run deterministic offline verification')}><span className="button-symbol">＋</span> Run offline demo</button>
        </header>

        <section className="project-scope" aria-label="Project scope">
          <label htmlFor="project-scope-select">Project scope</label>
          <select id="project-scope-select" value={activeProjectId} onChange={(event) => setActiveProjectId(event.target.value)}>
            <option value="">All projects</option>
            {projects.map((item) => <option value={item.id} key={item.id}>{item.display_name} · {item.canonical_path}</option>)}
          </select>
          <span>{activeProjectId ? `Showing ${activeProject?.display_name ?? 'selected project'} · ${totalRunCount ?? runs.length} runs` : `Showing all projects · ${totalRunCount ?? runs.length} runs${workspaceStatus?.local_only ? ' · local only' : ''}`}</span>
        </section>
        <form className="add-project-form" onSubmit={(event) => void addProject(event)} aria-label="Add project">
          <label htmlFor="add-project-path">Add an existing project</label>
          <input id="add-project-path" type="text" autoComplete="off" placeholder="Absolute path to a project folder" value={newProjectPath} onChange={(event) => setNewProjectPath(event.target.value)} aria-describedby={addProjectError ? 'add-project-error' : undefined} />
          <button className="button button-secondary" type="submit" disabled={addingProject || !token || !newProjectPath.trim()}>{addingProject ? 'Adding…' : 'Add project'}</button>
          {addProjectError && <p id="add-project-error" className="form-error" role="alert">{addProjectError}</p>}
        </form>
        {activeProject && <div className={`project-readiness ${activeProject.readiness?.status === 'READY' ? 'ready' : ''}`} role="status">
          <strong>{activeProject.display_name} · {activeProject.readiness?.status ?? 'READINESS UNKNOWN'}</strong>
          <span>{activeProject.readiness?.reason ?? 'Refresh project status to assess configuration.'}</span>
          {activeProject.readiness?.action && <span>{activeProject.readiness.action}</span>}
          {activeProject.readiness?.status === 'MISSING_CONFIGURATION' || activeProject.readiness?.status === 'MISSING_ROUTE' ? <button className="text-button" onClick={() => setView('routing')}>Review route settings →</button> : null}
          {activeProject.readiness?.status === 'MISSING_PROFILE' || activeProject.readiness?.status === 'PROFILE_NOT_AUTHORIZED' || activeProject.readiness?.status === 'CAPABILITY_NOT_VERIFIED' ? <button className="text-button" onClick={() => setView('profiles')}>Review provider profiles →</button> : null}
        </div>}

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
          <section className="panel history-panel"><div className="panel-head"><div><div className="eyebrow">PERSISTED RUNS</div><h2>All runs</h2></div><span className="count">{runs.length} total</span></div><RunList runs={runs} selected={selected} onSelect={setSelected} disconnected={connection === 'disconnected'} /></section>
        ) : (
          <>
            <section className="overview-grid">
              <div className="panel runs-panel"><div className="panel-head"><div><div className="eyebrow">WORKSPACE ACTIVITY</div><h2>Recent runs</h2></div><button className="text-button" onClick={() => setView('history')}>View history <span>↗</span></button></div><RunList runs={runs.slice(0, 6)} selected={selected} onSelect={setSelected} disconnected={connection === 'disconnected'} /></div>
              <div className="panel start-panel"><div className="panel-head"><div><div className="eyebrow">CONFIGURED WORKFLOW</div><h2>Start a workflow</h2></div><span className="small-icon">↗</span></div>
                <form onSubmit={(event) => { event.preventDefault(); void startRun() }}>
                  <label>Project path<input required value={project} onChange={(event) => setProject(event.target.value)} /></label>
                  <label>Task prompt<textarea required rows={3} value={prompt} onChange={(event) => setPrompt(event.target.value)} /></label>
                  <button className="button button-secondary" disabled={busy || !token}>{busy ? 'Starting…' : 'Start workflow'}<span>→</span></button>
                </form>
                <div className="inline-note"><span className="sim-dot" />Execution labels follow configured routes and persisted evidence.</div>
              </div>
            </section>

            <section className="panel detail-panel" aria-label="Run detail surface" aria-live="polite">
              <div className="panel-head"><div><div className="eyebrow">OBSERVED WORKFLOW</div><h2>{selectedRun ? <>Run <code>{selectedRun.id.slice(0, 8)}</code></> : 'Run detail'}</h2></div>{selectedRun && <RunDetailStatus run={selectedRun} onPause={() => void runControl('pause')} onResume={() => void runControl('resume')} onCancel={() => void cancelRun()} />}</div>
              {!report ? <div className="empty-inline">Select a run to inspect its stages, routes, and evidence.</div> : <>
                <div className="run-context"><span className="context-label">EXECUTION</span><span className={`execution-tag ${(report.acceptance?.execution ?? report.run.execution ?? 'UNKNOWN').toLowerCase()}`}>{report.acceptance?.execution ?? report.run.execution ?? 'UNKNOWN'}</span><span className="context-label">ACCEPTANCE</span><span>{report.acceptance?.verdict ?? 'NOT_RECORDED'}</span></div>
                <div className="run-context"><span className="context-label">TASK</span><span>{report.run.prompt}</span><span className="context-label">PROJECT</span><code>{report.run.project}</code></div>
                <div className="stage-list">{report.stages.map((stage, index) => {
                  const latest = (report.execution_attempts ?? []).filter((attempt) => attempt.stage === stage.key).at(-1)
                  return <div className="stage-row" key={stage.id}><span className={stage.state === 'SUCCEEDED' ? 'stage-index complete' : 'stage-index'} aria-hidden="true">{stage.state === 'SUCCEEDED' ? '✓' : String(index + 1).padStart(2, '0')}</span><strong>{stage.key.replaceAll('_', ' ')}</strong><span className="stage-route">{stage.provider} <i>/</i> {stage.profile} <i>·</i> {latest?.model || stage.model || 'default'} <i>·</i> {latest?.effort || stage.effort || 'default effort'} <i>·</i> {latest ? `${latest.phase} iteration ${latest.iteration}` : `attempt ${stage.attempt}`}</span><span className={`stage-state ${stage.state === 'SUCCEEDED' ? 'good' : ''}`}>{stage.state}</span></div>
                })}</div>
                <ExecutionHistoryPanel attempts={report.execution_attempts ?? []} qualityExecutions={report.quality_executions ?? []} />
                {!!report.routing_decisions?.length && <details className="route-details"><summary>Routing decisions <span>{report.routing_decisions.length} stages</span></summary><div className="route-list">{report.routing_decisions.map((decision) => <div key={decision.stage}><strong>{decision.stage}</strong><span>{decision.provider} / {decision.profile}</span><small>{decision.explanation}</small></div>)}</div></details>}
              </>}
            </section>

            <section className="evidence-grid">
              <div className="panel evidence-panel"><div className="panel-head"><div><div className="eyebrow">DURABLE EVENTS</div><h2>Activity stream</h2></div><span className={`live-label ${connection === 'disconnected' ? 'offline' : ''}`} role="status" aria-label={`Event stream ${!selected ? 'idle' : connection}`}><i /> {!selected ? 'SELECT RUN' : connection === 'disconnected' ? 'RECONNECTING' : 'LIVE'}</span></div><EventList events={events} /></div>
              <div className="panel evidence-panel"><div className="panel-head"><div><div className="eyebrow">RECORDED VERDICTS</div><h2>Quality gates</h2></div><span className="count">{report?.gates.length ?? 0}</span></div><GateList gates={report?.gates ?? []} /></div>
            </section>
            {report && <section className="panel artifacts-panel"><div className="panel-head"><div><div className="eyebrow">RUN OUTPUTS</div><h2>Artifacts &amp; approvals</h2></div></div>{report.artifacts?.length || report.approvals?.length ? <div className="artifact-list">{report.artifacts?.map((artifact) => <div className="artifact-row" key={artifact.id}><span>▤</span><strong>{artifact.path}</strong><code>{artifact.sha256.slice(0, 12)}</code></div>)}<ApprovalActions approvals={report.approvals ?? []} runState={report.run.state} busy={approvalBusy} onResolve={(id, decision) => void resolveApproval(id, decision)} /></div> : <div className="empty-inline">No artifacts or approvals recorded for this run.</div>}</section>}
          </>
        )}
        <footer className="footer"><span>SpecTide <b>{appVersion}</b></span><span>Run labels come from persisted route configuration and stage evidence</span></footer>
      </main>
    </div>
  )
}

export function ApprovalActions({ approvals, runState, busy = '', onResolve }: { approvals: Approval[]; runState: Run['state']; busy?: string; onResolve: (id: string, decision: 'approved' | 'denied') => void }) {
  return <>{approvals.map((approval) => <div className="artifact-row" key={approval.id}><span aria-hidden="true">◇</span><strong>{approval.action}</strong><span className="state-pill quiet">{approval.status}</span>{approval.status === 'PENDING' && runState === 'PAUSED' && <span className="detail-actions"><button className="button button-primary control-button" aria-label={`Approve ${approval.action}`} disabled={Boolean(busy)} onClick={() => onResolve(approval.id, 'approved')}>Approve write</button><button className="button button-danger" aria-label={`Deny ${approval.action}`} disabled={Boolean(busy)} onClick={() => onResolve(approval.id, 'denied')}>Deny</button></span>}</div>)}</>
}

export function RunList({ runs, selected, onSelect, disconnected = false }: { runs: Run[]; selected: string; onSelect: (id: string) => void; disconnected?: boolean }) {
  if (!runs.length) return <><div className="empty-state compact" role="status" aria-label="Empty run list"><span className="empty-icon" aria-hidden="true">⌁</span><strong>No runs recorded yet</strong><p>Start a configured workflow or run the offline demo.</p></div>{disconnected && <SharedStateSurface state="disconnected" message="Run history may be stale while the local API is unavailable." />}</>
  return <>{disconnected && <SharedStateSurface state="disconnected" message="Run history may be stale while the local API is unavailable." />}<div className="run-list" aria-label="Run list">{runs.map((run) => <button key={run.id} className={`run-card ${selected === run.id ? 'selected' : ''}`} onClick={() => onSelect(run.id)}><span className="run-avatar" aria-hidden="true">{run.state === 'COMPLETED' ? '✓' : run.state === 'RUNNING' ? '◌' : '·'}</span><span className="run-copy"><strong>{run.prompt}</strong><span>{run.project} <i aria-hidden="true">·</i> {new Date(run.created_at).toLocaleString()}</span></span>{run.execution && <span className={`execution-tag ${run.execution.toLowerCase()}`}>{run.execution}</span>}<span className={`state-pill ${runTone(run.state)}`}>{run.state}</span></button>)}</div></>
}

export function RunDetailStatus({ run, onPause, onResume, onCancel }: { run: Run; onPause: () => void; onResume: () => void; onCancel: () => void }) {
  return <div className="detail-actions"><span className={`state-pill ${runTone(run.state)}`}>{run.state}</span>{run.state === 'RUNNING' && <button className="button button-secondary control-button" onClick={onPause}>Pause</button>}{run.state === 'PAUSED' && <button className="button button-secondary control-button" onClick={onResume}>Resume</button>}{['RUNNING', 'PAUSED'].includes(run.state) && <button className="button button-danger" onClick={onCancel}>Cancel run</button>}</div>
}

export function EventList({ events }: { events: LocalEvent[] }) {
  if (!events.length) return <div className="empty-inline">Select a run to follow its persisted event stream.</div>
  return <div className="event-list">{events.map((event, index) => {
    const payload = event.payload ?? {}
    const summaries = [payload.summary, payload.provider_event_type, payload.result, payload.verdict, payload.phase, payload.iteration != null ? `iteration ${payload.iteration}` : undefined, payload.dropped_count != null ? `${payload.dropped_count} events dropped` : undefined].filter((value): value is string => typeof value === 'string' && value.length > 0)
    return <div className="event-row" key={`${event.id}-${index}`}><time>{event.timestamp ? new Date(event.timestamp).toLocaleTimeString() : '—'}</time><span className="event-dot" /><span className="event-copy"><strong>{event.type.replaceAll('.', ' ')}</strong>{event.stage && <small>{event.stage}</small>}{summaries.map((summary, summaryIndex) => <small key={`${summaryIndex}-${summary}`}>{summary}</small>)}</span></div>
  })}</div>
}

export function ExecutionHistoryPanel({ attempts, qualityExecutions }: { attempts: ExecutionAttempt[]; qualityExecutions: QualityExecution[] }) {
  const active = attempts.filter((attempt) => attempt.result === 'RUNNING').at(-1)
  return <section className="execution-history" aria-label="Provider and quality execution history">
    <h3>Provider invocations</h3>
    {!attempts.length ? <div className="empty-inline">No provider invocation history is recorded.</div> : <div className="stage-list">{attempts.map((attempt) => <div className="stage-row" key={attempt.id} data-active={attempt.id === active?.id || undefined}><span className="stage-index" aria-hidden="true">{attempt.iteration}</span><strong>{attempt.phase} · {attempt.result === 'RUNNING' ? 'ACTIVE' : attempt.result}</strong><span className="stage-route">{attempt.provider} / {attempt.profile} · {attempt.model || 'default'} · {attempt.effort || 'default effort'} · iteration {attempt.iteration}</span><span className="stage-state">{attempt.duration_ms} ms</span></div>)}</div>}
    <h3>Quality execution history</h3>
    {!qualityExecutions.length ? <div className="empty-inline">No local quality execution history is recorded.</div> : <div className="gate-list">{qualityExecutions.map((execution) => <article className="gate-row" key={execution.id}><span className="gate-symbol">{execution.verdict === 'PASS' ? '✓' : '!'}</span><span className="gate-name"><strong>{execution.phase} · iteration {execution.iteration} · {execution.verdict}</strong>{execution.results.map((result) => <small key={result.key}>{result.key}: {result.verdict}{result.summary ? ` · ${result.summary}` : ''}</small>)}</span><span className={`gate-verdict ${execution.verdict === 'PASS' ? 'good' : ''}`}>{execution.results.length} checks</span></article>)}</div>}
  </section>
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
  return <div className="route-preview-results" role="status" aria-label="Ready route preview"><p><strong>Ready · NOT RUN</strong><span>Mode: {preview.mode} · Execution: {preview.execution}</span></p><div className="route-list">{preview.decisions.map((decision) => <div key={decision.stage}><strong>{decision.stage.replaceAll('_', ' ')}</strong><span>{decision.route.provider} / {decision.route.profile} · {decision.route.model || 'default model'} · {decision.route.effort || 'default effort'}</span><small>{decision.explanation}</small></div>)}</div></div>
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
