import { describe, expect, it } from 'vitest'

import {
	planRollbackMessage,
	planStageMessage,
	processPlanMessage,
	replanAllStagesMessage,
} from './plan-message'

describe('plan messages', () => {
	// The orchestrator routes on these sentences, so each has to say plainly
	// which work it is asking for.
	it('asks for the whole plan, and says so', () => {
		expect(processPlanMessage()).toContain('every stage')
		expect(processPlanMessage()).toContain('rollback')
	})

	it('distinguishes a replan from a first plan', () => {
		expect(replanAllStagesMessage()).not.toEqual(processPlanMessage())
		expect(replanAllStagesMessage()).toContain('Replan')
	})

	// A stage is matched by the title the DAG spells, so the title has to survive
	// verbatim -- quotes and all.
	it('embeds the stage title verbatim', () => {
		expect(planStageMessage('Deploy Centrifugo')).toContain(
			'"Deploy Centrifugo"',
		)
		expect(planStageMessage('Add the "prod" alert')).toContain(
			'Add the "prod" alert',
		)
	})

	it('asks for the rollback alone without naming a stage', () => {
		const message = planRollbackMessage()
		expect(message).toContain('rollback')
		expect(message).toContain('Leave the stages alone')
	})
})
