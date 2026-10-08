import type { IncidentRun, IncidentStep } from '../api/incidents'

export interface InvestigationProgress {
	total: number
	done: number
	failed: number
	running: boolean
	/** One line for the card header. */
	headline: string
}

/** The label of one step, as the progress list shows it. */
export function stepLabel(step: IncidentStep): string {
	switch (step.kind) {
		case 'digger':
			return step.environment ? `${step.system} · ${step.environment}` : (step.system ?? 'System')
		case 'synthesis':
			return 'Synthesis: hypothesis, root causes, actions'
		case 'impact':
			return 'Impact assessment'
		case 'closure':
			return 'Closure summary'
	}
}

export function investigationProgress(run: IncidentRun): InvestigationProgress {
	const total = run.steps.length
	const done = run.steps.filter((s) => s.status === 'done').length
	const failed = run.steps.filter((s) => s.status === 'failed').length
	const running = run.status === 'running'
	const noun = run.kind === 'closure' ? 'Closing the incident' : 'Investigation'

	let headline: string
	if (running) {
		headline = `${noun}: ${done} of ${total} steps done`
	} else if (run.status === 'failed') {
		headline = `${noun} stopped${run.error ? ` — ${run.error}` : ''}`
	} else if (failed > 0) {
		headline = `${noun} finished; ${failed} of ${total} steps did not`
	} else {
		headline = `${noun} finished`
	}
	return { total, done, failed, running, headline }
}
