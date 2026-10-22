import { describe, expect, it } from 'vitest'
import { runTone, summarizeRuns, type Run } from './domain'

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
