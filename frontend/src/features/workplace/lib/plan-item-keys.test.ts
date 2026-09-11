import { describe, expect, it } from 'vitest'
import type { ActionPlan } from '@/features/dialogs/api/dialogs'
import { buildPlanItemKeys } from './plan-item-keys'

const plan: ActionPlan = {
	stages: [
		{
			number: 1,
			title: 'Deploy',
			description: 'Deploy the service.',
			steps: [
				{ type: 'shell', action: 'apply manifests' },
				{ type: 'code', action: 'bump the image tag' },
			],
			checks: [{ check: 'pods are ready', expectation: 'all Running' }],
		},
		{
			number: 2,
			title: 'Verify',
			description: 'Verify the rollout.',
			steps: [{ type: 'shell', action: 'curl the healthcheck' }],
			checks: [],
		},
	],
	rollback: [{ type: 'shell', action: 'roll the deployment back' }],
}

describe('buildPlanItemKeys', () => {
	it('returns nothing for a missing plan', () => {
		expect(buildPlanItemKeys(null)).toEqual([])
	})

	it('returns steps before checks, stage by stage', () => {
		expect(buildPlanItemKeys(plan)).toEqual([
			's0.step0',
			's0.step1',
			's0.check0',
			's1.step0',
		])
	})

	it('leaves rollback items out, matching backend progress counting', () => {
		expect(
			buildPlanItemKeys(plan).some((key) => key.startsWith('rollback')),
		).toBe(false)
	})

	it('returns nothing for a plan with no stages', () => {
		expect(buildPlanItemKeys({ stages: [], rollback: [] })).toEqual([])
	})
})
