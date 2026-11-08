import type {
	ActionExecRun,
	CommandRun,
	CommandRunStatus,
} from '@/features/dialogs/api/dialogs'

/**
 * Whether a row can have commands to show. A container run is the coding
 * agent, whose commands never pass through nib; everything else is an action
 * sub-agent, which runs them with run_command.
 */
export function canViewCommands(run: ActionExecRun | undefined): boolean {
	return run !== undefined && run.kind !== 'container'
}

const STATUS_LABELS: Record<CommandRunStatus, string> = {
	running: 'Running',
	succeeded: 'Succeeded',
	failed: 'Failed',
	timed_out: 'Timed out',
	cancelled: 'Cancelled',
	lost: 'Lost in a restart',
}

/** A short label for a run's outcome, with the exit code where it says more. */
export function commandRunLabel(run: CommandRun): string {
	const label = STATUS_LABELS[run.status] ?? run.status
	if (run.status === 'failed' && run.exit_code !== undefined) {
		return `${label} (exit ${run.exit_code})`
	}
	return label
}

/** The first non-blank line of a script, which is what a list row shows. */
export function scriptSummary(script: string, max = 120): string {
	const line =
		script
			.split('\n')
			.map((l) => l.trim())
			.find((l) => l !== '') ?? ''
	const more = script.trim().includes('\n') ? ' …' : ''
	if (line.length > max) return `${line.slice(0, max - 1)}…`
	return line + more
}

/** A duration in ms as the largest unit or two that matter. */
export function formatDuration(ms: number): string {
	if (ms < 1000) return `${ms} ms`
	const s = Math.round(ms / 1000)
	if (s < 60) return `${s} s`
	const m = Math.floor(s / 60)
	if (m < 60) return `${m} min ${s % 60} s`
	return `${Math.floor(m / 60)} h ${m % 60} min`
}

/**
 * Appends a page of output to what is already shown. A page that does not
 * start where the shown text ends (a re-read after an error) replaces nothing
 * and is dropped, so text is never doubled.
 */
export function appendOutput(
	shown: { text: string; next: number },
	page: { output: string; offset: number; next_offset: number },
): { text: string; next: number } {
	if (page.offset !== shown.next) return shown
	return { text: shown.text + page.output, next: page.next_offset }
}
