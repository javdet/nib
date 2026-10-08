import { describe, expect, it } from 'vitest'
import type { Postmortem } from '../api/incidents'
import { isActionSelectable, pruneSelection } from './action-selection'

const doc = {
	shortTerm: [
		{ id: 'st-1', text: 'a', status: 'todo' },
		{ id: 'st-2', text: 'b', status: 'in_plan', planId: 'p' },
		{ id: 'st-3', text: 'c', status: 'done' },
	],
	longTerm: [{ id: 'lt-4', text: 'd', status: 'todo' }],
} as unknown as Postmortem

describe('action selection', () => {
	it('selects only actions still to do', () => {
		expect(doc.shortTerm.map(isActionSelectable)).toEqual([true, false, false])
	})

	it('prunes ids that are gone or no longer selectable', () => {
		const pruned = pruneSelection(new Set(['st-1', 'st-2', 'st-3', 'lt-4', 'st-9']), doc)
		expect([...pruned].sort()).toEqual(['lt-4', 'st-1'])
	})
})
