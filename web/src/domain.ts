export type RunState = 'CREATED' | 'RUNNING' | 'PAUSED' | 'COMPLETED' | 'BLOCKED' | 'FAILED' | 'CANCELLED'

export interface Run {
  id: string
  project_id?: string
  project: string
  prompt: string
  state: RunState
  execution?: 'SIMULATED' | 'LIVE' | 'MIXED' | 'UNKNOWN'
  created_at: string
}

export interface Project {
  id: string
  canonical_path: string
  display_name: string
  readiness?: { status: string; reason: string; action?: string }
}

export type PreviewBlockerCategory =
  | 'MISSING_ROUTE'
  | 'MISSING_PROFILE'
  | 'PROFILE_NOT_AUTHORIZED'
  | 'CAPABILITY_NOT_VERIFIED'
  | 'PROJECT_UNAVAILABLE'
  | 'CONFIGURATION_INVALID'
  | 'UNKNOWN'

export type PreviewRemediationTarget = 'routing' | 'profiles' | 'readiness' | 'project-selection'

export interface PreviewRemediation {
  target: PreviewRemediationTarget
  label: string
}

export interface PreviewBlocker {
  category: PreviewBlockerCategory
  message: string
  remediation: PreviewRemediation
  stage?: string
  profile?: string
  provider?: string
}

export interface DecodedApiFailure {
  error?: { code?: string; message?: string; request_id?: string }
  execution?: 'NOT_RUN'
  dry_run?: boolean
  preview_blockers: PreviewBlocker[]
}

const previewBlockerCategories: PreviewBlockerCategory[] = [
  'MISSING_ROUTE', 'MISSING_PROFILE', 'PROFILE_NOT_AUTHORIZED', 'CAPABILITY_NOT_VERIFIED',
  'PROJECT_UNAVAILABLE', 'CONFIGURATION_INVALID', 'UNKNOWN',
]
const remediationTargets: PreviewRemediationTarget[] = ['routing', 'profiles', 'readiness', 'project-selection']

export function decodePreviewApiFailure(body: string): DecodedApiFailure | null {
  try {
    const value: unknown = JSON.parse(body)
    if (!value || typeof value !== 'object') return null
    const record = value as Record<string, unknown>
    if (!Array.isArray(record.preview_blockers) || record.preview_blockers.length !== 1) return null
    const item = record.preview_blockers[0] as Record<string, unknown> | null
    if (!item || typeof item !== 'object' || !previewBlockerCategories.includes(item.category as PreviewBlockerCategory) || typeof item.message !== 'string') return null
    const remediation = item.remediation as Record<string, unknown> | null
    if (!remediation || typeof remediation !== 'object' || !remediationTargets.includes(remediation.target as PreviewRemediationTarget) || typeof remediation.label !== 'string') return null
    const safeOptional = (field: string) => typeof item[field] === 'string' && /^[a-zA-Z0-9._-]{1,80}$/.test(item[field] as string) ? item[field] as string : undefined
    const blocker: PreviewBlocker = {
      category: item.category as PreviewBlockerCategory,
      message: item.message.slice(0, 240),
      remediation: { target: remediation.target as PreviewRemediationTarget, label: remediation.label.slice(0, 80) },
      ...(safeOptional('stage') ? { stage: safeOptional('stage') } : {}),
      ...(safeOptional('profile') ? { profile: safeOptional('profile') } : {}),
      ...(safeOptional('provider') ? { provider: safeOptional('provider') } : {}),
    }
    const apiError = record.error && typeof record.error === 'object' ? record.error as Record<string, unknown> : undefined
    return {
      preview_blockers: [blocker],
      ...(apiError ? { error: {
        ...(typeof apiError.code === 'string' ? { code: apiError.code } : {}),
        ...(typeof apiError.message === 'string' ? { message: apiError.message.slice(0, 240) } : {}),
        ...(typeof apiError.request_id === 'string' ? { request_id: apiError.request_id.slice(0, 80) } : {}),
      } } : {}),
      dry_run: record.dry_run === true,
      execution: record.execution === 'NOT_RUN' ? 'NOT_RUN' : undefined,
    }
  } catch {
    return null
  }
}

export interface WorkspaceStatus {
  workspace_id: string
  version: string
  dashboard_url: string
  local_only: boolean
}

export interface Stage {
  id: string
  key: string
  state: string
  provider: string
  profile: string
  model: string
  effort: string
  attempt: number
}

export interface Gate {
  key: string
  verdict: string
  evidence: string
}

export interface Decision {
  stage: string
  provider: string
  profile: string
  model: string
  effort: string
  policy: string
  explanation: string
}

export interface Report {
  run: Run
  acceptance?: { verdict: string; execution: 'SIMULATED' | 'LIVE' | 'MIXED'; required_gates: Gate[]; simulation_gates?: Gate[] }
  stages: Stage[]
  execution_attempts?: ExecutionAttempt[]
  quality_executions?: QualityExecution[]
  gates: Gate[]
  routing_decisions: Decision[]
  artifacts: Array<{ id: string; stage: string; path: string; kind: string; sha256: string }>
  approvals: Approval[]
}

export interface ExecutionAttempt {
  id: string
  stage: string
  phase: string
  iteration: number
  provider: string
  profile: string
  model: string
  effort?: string
  started_at: string
  finished_at?: string
  duration_ms: number
  result: string
  exit_code: number
  error?: string
  artifact_ids?: string[]
}

export interface QualityExecution {
  id: string
  phase: string
  iteration: number
  started_at: string
  finished_at: string
  verdict: string
  results: QualityResult[]
}

export interface QualityResult {
  key: string
  verdict: string
  command: string[]
  duration_ns: number
  summary: string
  exit_code: number
}

export interface Approval {
  id: string
  action: string
  status: 'PENDING' | 'APPROVED' | 'DENIED' | string
  decision: string
}

export interface Profile {
  id: string
  name: string
  provider: string
  executable: string
  allowed_scopes: string[]
  capability?: {
    provider: string
    executable_found: boolean
    version: string
    authenticated: string
    read_only_execution: boolean
    workspace_write: boolean
  }
}

export function summarizeRuns(runs: Run[]) {
  return {
    active: runs.filter((run) => ['CREATED', 'RUNNING', 'PAUSED'].includes(run.state)).length,
    completed: runs.filter((run) => run.state === 'COMPLETED').length,
    total: runs.length,
  }
}

export function runTone(state: RunState) {
  switch (state) {
    case 'COMPLETED': return 'good'
    case 'FAILED':
    case 'BLOCKED': return 'bad'
    case 'RUNNING': return 'active'
    default: return 'quiet'
  }
}
