export type GuidanceTarget = 'routing' | 'profiles' | 'readiness' | 'project-selection' | 'preview' | 'retry'

export interface DashboardGuidance {
  title: string
  explanation: string
  target: GuidanceTarget
  actionLabel: string
  canPreview: boolean
  canStart: boolean
}

const guidanceByStatus: Record<string, DashboardGuidance> = {
  READY: {
    title: 'Ready for a dry route check',
    explanation: 'The selected project has configured routes and stored provider evidence.',
    target: 'preview', actionLabel: 'Preview routes', canPreview: true, canStart: true,
  },
  MISSING_CONFIGURATION: {
    title: 'Project configuration is missing',
    explanation: 'Add route configuration before checking or starting work.',
    target: 'routing', actionLabel: 'Review route settings', canPreview: false, canStart: false,
  },
  MISSING_ROUTE: {
    title: 'Routes need configuration',
    explanation: 'Configure a route before checking or starting work.',
    target: 'routing', actionLabel: 'Review route settings', canPreview: false, canStart: false,
  },
  INVALID_CONFIGURATION: {
    title: 'Project configuration needs attention',
    explanation: 'Review the route configuration before checking or starting work.',
    target: 'routing', actionLabel: 'Review route settings', canPreview: false, canStart: false,
  },
  MISSING_PROFILE: {
    title: 'A route profile is not registered',
    explanation: 'Register the configured provider profile before continuing.',
    target: 'profiles', actionLabel: 'Review profile setup', canPreview: false, canStart: false,
  },
  PROFILE_NOT_AUTHORIZED: {
    title: 'Profile access needs review',
    explanation: 'The configured profile is outside this project’s permitted scope.',
    target: 'profiles', actionLabel: 'Review profile access', canPreview: false, canStart: false,
  },
  CAPABILITY_NOT_VERIFIED: {
    title: 'Provider readiness is unverified',
    explanation: 'Verify the configured provider profile before continuing.',
    target: 'readiness', actionLabel: 'Review profile readiness', canPreview: false, canStart: false,
  },
  PATH_CHANGED: {
    title: 'The registered project is unavailable',
    explanation: 'Select an available project to continue. Your task draft stays in this page.',
    target: 'project-selection', actionLabel: 'Select an available project', canPreview: false, canStart: false,
  },
  PROJECT_UNAVAILABLE: {
    title: 'The registered project is unavailable',
    explanation: 'Select an available project to continue. Your task draft stays in this page.',
    target: 'project-selection', actionLabel: 'Select an available project', canPreview: false, canStart: false,
  },
  DISCONNECTED: {
    title: 'Current project readiness is unavailable',
    explanation: 'Reconnect to the local API before deciding which project action is needed.',
    target: 'retry', actionLabel: 'Retry after reconnecting', canPreview: false, canStart: false,
  },
}

export function dashboardGuidance(status?: string): DashboardGuidance {
  if (!status) return {
    title: 'Choose a project to prepare work',
    explanation: 'All-project scope is for history. Select one registered project to preview or start work.',
    target: 'project-selection', actionLabel: 'Select a project', canPreview: false, canStart: false,
  }
  return guidanceByStatus[status] ?? {
    title: 'Project readiness is unknown',
    explanation: 'Refresh project readiness before previewing or starting work.',
    target: 'readiness', actionLabel: 'Refresh project readiness', canPreview: false, canStart: false,
  }
}
