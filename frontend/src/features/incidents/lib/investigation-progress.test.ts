import { describe, expect, it } from 'vitest'
import type { IncidentRun } from '../api/incidents'
import { investigationProgress, stepLabel } from './investigation-progress'

const run: IncidentRun = {
	runId: 'r',
	kind: 'investigation',
	status: 'running',
	startedAt: 1,
	brief: '',
	steps: [
		{ kind: 'digger', system: 'api', environment: 'prod', status: 'done' },
		{ kind: 'digger', system: 'db', status: 'failed', error: 'x' },
		{ kind: 'synthesis', status: 'running' },
		{ kind: 'impact', status: 'pending' },
	],
}

describe('investigationProgress', () => {
	it('counts a running run', () => {
		expect(investigationProgress(run)).toEqual({
			total: 4,
			done: 1,
			failed: 1,
			running: true,
			headline: 'Investigation: 1 of 4 steps done',
		})
	})

	it('reports how a run ended', () => {
		expect(investigationProgress({ ...run, status: 'done' }).headline).toBe(
			'Investigation finished; 1 of 4 steps did not',
		)
		expect(
			investigationProgress({ ...run, kind: 'closure', status: 'failed', error: 'out of time' }).headline,
		).toBe('Closing the incident stopped — out of time')
	})

	it('labels steps', () => {
		expect(run.steps.map(stepLabel)).toEqual([
			'api · prod',
			'db',
			'Synthesis: hypothesis, root causes, actions',
			'Impact assessment',
		])
	})
})
