/**
 * The sentences the planning buttons put into the main chat.
 *
 * Pressing a button sends a message rather than calling an endpoint: the
 * orchestrator reads it and launches the plan sub-agent, which is the same path
 * a message typed by hand takes. That is what lets an operator write "let's work
 * on stage 1" in their own words, or in their own language, and get the same
 * behaviour as the button.
 *
 * See execute-action-message.ts, which does the same for the Execute buttons.
 */

/** Asks for the whole DAG to be worked out into a detailed action plan. */
export function processPlanMessage(): string {
	return 'Work the whole DAG into a detailed action plan: plan every stage, then the rollback.'
}

/** Asks for the existing action plan to be thrown away and rebuilt. */
export function replanAllStagesMessage(): string {
	return 'Replan every stage of the DAG from scratch, and redo the rollback with them.'
}

/** Asks for one stage to be replanned, named as the DAG names it. */
export function planStageMessage(stageTitle: string): string {
	return `Replan the stage "${stageTitle}" only, and redo the rollback with it.`
}

/** Asks for the rollback list to be rebuilt from the stages as they stand. */
export function planRollbackMessage(): string {
	return 'Redo the rollback plan from the stages as they stand. Leave the stages alone.'
}
