import { describe, expect, it } from 'vitest'
import { dashboardGuidance } from './dashboard-guidance'

describe('readiness guidance', () => {
  it.each([
    ['READY', 'preview', 'Preview routes'],
    ['MISSING_CONFIGURATION', 'routing', 'Review route settings'],
    ['MISSING_ROUTE', 'routing', 'Review route settings'],
    ['INVALID_CONFIGURATION', 'routing', 'Review route settings'],
    ['MISSING_PROFILE', 'profiles', 'Review profile setup'],
    ['PROFILE_NOT_AUTHORIZED', 'profiles', 'Review profile access'],
    ['CAPABILITY_NOT_VERIFIED', 'readiness', 'Review profile readiness'],
    ['PATH_CHANGED', 'project-selection', 'Select an available project'],
    ['PROJECT_UNAVAILABLE', 'project-selection', 'Select an available project'],
    ['DISCONNECTED', 'retry', 'Retry after reconnecting'],
    ['FUTURE_STATUS', 'readiness', 'Refresh project readiness'],
  ])('maps %s to one next action', (status, target, label) => {
    const guidance = dashboardGuidance(status)
    expect(guidance.target).toBe(target)
    expect(guidance.actionLabel).toBe(label)
    expect(guidance.actionLabel).toBeTruthy()
  })

  it('keeps all-project scope observational until a project is selected', () => {
    expect(dashboardGuidance(undefined)).toMatchObject({
      target: 'project-selection',
      actionLabel: 'Select a project',
      canPreview: false,
      canStart: false,
    })
  })
})
