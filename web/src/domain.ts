export type RunState = 'CREATED' | 'RUNNING' | 'PAUSED' | 'COMPLETED' | 'BLOCKED' | 'FAILED' | 'CANCELLED'

export interface Run {
  id: string
  project: string
  prompt: string
  state: RunState
  execution?: 'SIMULATED' | 'LIVE' | 'MIXED' | 'UNKNOWN'
  created_at: string
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
  gates: Gate[]
  routing_decisions: Decision[]
  artifacts: Array<{ id: string; stage: string; path: string; kind: string; sha256: string }>
  approvals: Approval[]
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
