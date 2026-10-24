import { describe, expect, it } from 'vitest'
import { decodePreviewApiFailure, runTone, summarizeRuns, type Run } from './domain'

const run = (state: Run['state'], id: string): Run => ({
  id,
  project: '/workspace',
  prompt: 'demo',
  state,
  created_at: '2026-10-22T00:00:00Z',
})

describe('dashboard run summaries', () => {
  it('counts active and completed states from persisted run data', () => {
    expect(summarizeRuns([run('RUNNING', '1'), run('PAUSED', '2'), run('COMPLETED', '3')]))
      .toEqual({ active: 2, completed: 1, total: 3 })
  })

  it('maps blocked and failed runs to the warning tone', () => {
    expect(runTone('BLOCKED')).toBe('bad')
    expect(runTone('FAILED')).toBe('bad')
    expect(runTone('COMPLETED')).toBe('good')
  })
})

describe('typed route-preview failures', () => {
  it('decodes a bounded blocker and optional safe route context', () => {
    expect(decodePreviewApiFailure(JSON.stringify({
      dry_run: true,
      execution: 'NOT_RUN',
      error: { code: 'ROUTE_PREVIEW_BLOCKED', message: 'generic', request_id: 'support-ref' },
      preview_blockers: [{ category: 'MISSING_PROFILE', message: 'A profile is needed.', remediation: { target: 'profiles', label: 'Review profile setup' }, stage: 'constitution', profile: 'local-codex', provider: 'codex' }],
    }))).toMatchObject({ preview_blockers: [{ category: 'MISSING_PROFILE', stage: 'constitution', profile: 'local-codex', provider: 'codex' }], execution: 'NOT_RUN' })
  })

  it('rejects unknown categories, remediation targets, and unbounded arrays', () => {
    const blocker = { category: 'RAW_ERROR', message: '/private/path', remediation: { target: 'shell', label: 'Run command' } }
    expect(decodePreviewApiFailure(JSON.stringify({ preview_blockers: [blocker] }))).toBeNull()
    expect(decodePreviewApiFailure(JSON.stringify({ preview_blockers: [blocker, blocker] }))).toBeNull()
    expect(decodePreviewApiFailure('{invalid')).toBeNull()
  })
})
