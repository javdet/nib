import { describe, expect, it } from 'vitest'
import type { ActionExecRun, CommandRun } from '@/features/dialogs/api/dialogs'
import {
	appendOutput,
	canViewCommands,
	commandRunLabel,
	formatDuration,
	scriptSummary,
} from './command-runs'

function execRun(overrides: Partial<ActionExecRun> = {}): ActionExecRun {
	return { status: 'done', startedAt: 1, attempt: 1, ...overrides }
}

function commandRun(overrides: Partial<CommandRun> = {}): CommandRun {
	return {
		id: 'r1',
		plan_id: 'p',
		dialog_id: 'd',
		mode: 'execute',
		kind: 'run',
		script: 'echo hi',
		isolated: true,
		uid: 20000,
		status: 'succeeded',
		output_bytes: 3,
		output_capped: false,
		started_at: '2026-11-08T00:00:00Z',
		duration_ms: 10,
		...overrides,
	}
}

describe('canViewCommands', () => {
	it('allows a sub-agent run, finished or not', () => {
		expect(canViewCommands(execRun({ kind: 'subagent' }))).toBe(true)
		expect(canViewCommands(execRun({ kind: 'subagent', status: 'running' }))).toBe(true)
	})

	it('allows a record from before runs carried a kind', () => {
		expect(canViewCommands(execRun({ kind: undefined }))).toBe(true)
	})

	it('refuses a container run and a row that never ran', () => {
		expect(canViewCommands(execRun({ kind: 'container' }))).toBe(false)
		expect(canViewCommands(undefined)).toBe(false)
	})
})

describe('commandRunLabel', () => {
	it('adds the exit code to a failure', () => {
		expect(commandRunLabel(commandRun({ status: 'failed', exit_code: 2 }))).toBe(
			'Failed (exit 2)',
		)
	})

	it('names a run a restart stranded', () => {
		expect(commandRunLabel(commandRun({ status: 'lost' }))).toBe('Lost in a restart')
	})
})

describe('scriptSummary', () => {
	it('shows the first non-blank line and marks that more follows', () => {
		expect(scriptSummary('\n  kubectl get pods\nkubectl get svc')).toBe(
			'kubectl get pods …',
		)
	})

	it('cuts a long line', () => {
		expect(scriptSummary('x'.repeat(10), 5)).toBe('xxxx…')
	})
})

describe('formatDuration', () => {
	it.each([
		[500, '500 ms'],
		[4_200, '4 s'],
		[125_000, '2 min 5 s'],
		[3_720_000, '1 h 2 min'],
	])('%i ms is %s', (ms, want) => {
		expect(formatDuration(ms)).toBe(want)
	})
})

describe('appendOutput', () => {
	it('appends a page that continues the shown text', () => {
		expect(
			appendOutput({ text: 'ab', next: 2 }, { output: 'cd', offset: 2, next_offset: 4 }),
		).toEqual({ text: 'abcd', next: 4 })
	})

	it('drops a page from anywhere else', () => {
		expect(
			appendOutput({ text: 'ab', next: 2 }, { output: 'ab', offset: 0, next_offset: 2 }),
		).toEqual({ text: 'ab', next: 2 })
	})
})
