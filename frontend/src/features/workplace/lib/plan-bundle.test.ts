import { describe, expect, it } from 'vitest'
import { parsePlanBundle, planBundleFileName } from './plan-bundle'

describe('planBundleFileName', () => {
	it('uses the plan title with the .nib extension', () => {
		expect(planBundleFileName('Deploy API v2!')).toBe('deploy-api-v2.nib')
	})

	it('falls back to plan for an empty title', () => {
		expect(planBundleFileName('   ')).toBe('plan.nib')
	})
})

describe('parsePlanBundle', () => {
	it('accepts a current plan file', () => {
		const bundle = parsePlanBundle(
			JSON.stringify({ format: 'nib-plan', version: 1, plan: { title: 'x' } }),
		)
		expect(bundle.plan).toEqual({ title: 'x' })
	})

	it('rejects text that is not JSON', () => {
		expect(() => parsePlanBundle('# a markdown export')).toThrow(
			/not valid JSON/,
		)
	})

	it('rejects JSON of another format', () => {
		expect(() => parsePlanBundle('{"format":"other","version":1}')).toThrow(
			/not a nib plan/,
		)
		expect(() => parsePlanBundle('[]')).toThrow(/not a nib plan/)
	})

	it('asks for an upgrade on a newer version', () => {
		expect(() =>
			parsePlanBundle('{"format":"nib-plan","version":2}'),
		).toThrow(/newer version/)
	})
})
