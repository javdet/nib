import { describe, expect, it } from 'vitest'
import {
	actionPlanItemNumber,
	actionPlanNumberForKey,
	actionPlanStageNumber,
	type ActionPlanItemKind,
} from './action-plan-number'

describe('actionPlanStageNumber', () => {
	it.each([
		[0, '1'],
		[1, '2'],
		[9, '10'],
	])('labels stage index %i as %s', (stageIdx, expected) => {
		expect(actionPlanStageNumber(stageIdx)).toBe(expected)
	})
})

describe('actionPlanItemNumber', () => {
	it.each<[ActionPlanItemKind, number, number, string]>([
		['step', 0, 0, '1.1'],
		['step', 0, 1, '1.2'],
		['step', 1, 2, '2.3'],
		['step', 9, 11, '10.12'],
		['check', 0, 0, '1.C1'],
		['check', 2, 1, '3.C2'],
		['rollback', 0, 0, 'R1'],
		['rollback', 0, 9, 'R10'],
	])('labels %s at stage %i index %i as %s', (kind, stageIdx, itemIdx, expected) => {
		expect(actionPlanItemNumber(kind, stageIdx, itemIdx)).toBe(expected)
	})

	it('ignores the stage index of a rollback entry, which is plan-level', () => {
		expect(actionPlanItemNumber('rollback', 5, 0)).toBe('R1')
	})
})

describe('actionPlanNumberForKey', () => {
	it.each([
		['s0.step0', '1.1'],
		['s0.step1', '1.2'],
		['s1.step0', '2.1'],
		['s0.check0', '1.C1'],
		['s2.check1', '3.C2'],
		['rollback.0', 'R1'],
		['rollback.3', 'R4'],
	])('labels %s as %s', (key, expected) => {
		expect(actionPlanNumberForKey(key)).toBe(expected)
	})

	it.each(['', 'nonsense', 's0.other0', 'rollback', 'sX.step0'])(
		'returns an empty label for %s',
		(key) => {
			expect(actionPlanNumberForKey(key)).toBe('')
		},
	)
})
