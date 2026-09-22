import type { FanoutStageStatus } from '@/features/dialogs/api/dialogs'

/**
 * Reports a fan-out stage's status the way an operator reads it.
 *
 * Only `awaiting_input` is reworded: it is the one status that asks something of
 * the person looking at it, and the raw value reads like a machine state rather
 * than a request.
 */
export function fanoutStageLabel(status: FanoutStageStatus): string {
	return status === 'awaiting_input' ? 'waiting for you' : status
}

export function fanoutStageClass(status: FanoutStageStatus): string {
	switch (status) {
		case 'failed':
			return 'text-destructive'
		case 'done':
			return 'text-lime-700 dark:text-lime-300'
		case 'awaiting_input':
			return 'text-amber-600 dark:text-amber-400'
		default:
			return ''
	}
}
