import type { ActionExecRun } from '@/features/dialogs/api/dialogs'
import type { ExecutorType } from '@/features/executor/api/executor'

/**
 * Whether the container logs of an action row can be read.
 *
 * Only while the row runs: the container outlives the run, but by then its
 * account of the work is in the execute chat. Only for the local docker
 * executor, which is the only one the backend reads logs from.
 */
export function canViewContainerLogs(
	run: ActionExecRun | undefined,
	executorType: ExecutorType | null,
): boolean {
	return (
		run?.status === 'running' &&
		run.kind === 'container' &&
		executorType === 'local'
	)
}
