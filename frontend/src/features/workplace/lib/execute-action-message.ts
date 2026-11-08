import type { ActionExecRun } from '@/features/dialogs/api/dialogs'

/**
 * The sentence the Execute button puts into the planning chat.
 *
 * Pressing it sends a message rather than calling an endpoint: the agent
 * reads it and calls `execute_action`, which is the same path a message typed by
 * hand takes. That is what lets an operator write "run step 1.1" in their own
 * words, or in their own language, and get the same behaviour as the button.
 */

/** Asks the agent to run one plan item. */
export function executeActionMessage(number: string): string {
	return `Execute plan item ${number}`
}

/**
 * Asks the agent to carry on with a plan item's last run, in that run's own
 * transcript, rather than start it over.
 */
export function continueActionMessage(number: string): string {
	return `Continue plan item ${number} from where its last run stopped`
}

/**
 * Whether a row's last run can be continued: a sub-agent run that ended
 * without finishing. A finished run has nothing left to do, a running one is
 * still going, and a container run keeps no transcript in nib to continue.
 */
export function canContinueRun(run: ActionExecRun | undefined): boolean {
	if (!run || run.kind === 'container') return false
	return (
		run.status === 'failed' ||
		run.status === 'cancelled' ||
		run.status === 'blocked'
	)
}
