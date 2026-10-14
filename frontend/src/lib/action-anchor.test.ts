import { describe, expect, it } from 'vitest'
import {
	actionAnchorId,
	actionHref,
	actionKeyFromHash,
	actionLinkFromHref,
	actionPagePath,
} from './action-anchor'

const PLAN = '0199c0de-0000-7000-8000-000000000001'

describe('actionKeyFromHash', () => {
	it.each([
		['#action-s3.step1', 's3.step1'],
		['#action-s0.check0', 's0.check0'],
		['#action-rollback.2', 'rollback.2'],
		['action-s0.step0', 's0.step0'],
	])('reads %s', (hash, key) => {
		expect(actionKeyFromHash(hash)).toBe(key)
	})

	it.each(['', '#', '#action-', '#action-nonsense', '#action-s0.other0', '#s0.step0', '#dialog:abc'])(
		'ignores %j',
		(hash) => {
			expect(actionKeyFromHash(hash)).toBeNull()
		},
	)

	it('ignores no hash at all', () => {
		expect(actionKeyFromHash(undefined)).toBeNull()
	})

	it('round-trips an anchor it built', () => {
		expect(actionKeyFromHash(`#${actionAnchorId('s1.check2')}`)).toBe('s1.check2')
	})
})

describe('actionLinkFromHref', () => {
	it('reads the plan and the row out of a chat link', () => {
		expect(actionLinkFromHref(`#action:${PLAN}:s3.step1`)).toEqual({ planId: PLAN, key: 's3.step1' })
	})

	it('round-trips a href it built', () => {
		expect(actionLinkFromHref(actionHref(PLAN, 'rollback.0'))).toEqual({
			planId: PLAN,
			key: 'rollback.0',
		})
	})

	it.each([
		undefined,
		'https://example.com',
		'#dialog:abc',
		'#action-s0.step0',
		'#action:',
		`#action:${PLAN}`,
		`#action::s0.step0`,
		`#action:${PLAN}:nonsense`,
	])('ignores %j', (href) => {
		expect(actionLinkFromHref(href)).toBeNull()
	})
})

describe('actionPagePath', () => {
	it('opens the plan with the row focused', () => {
		expect(actionPagePath(PLAN, 's3.step1')).toBe(`/workplace/${PLAN}#action-s3.step1`)
	})
})
