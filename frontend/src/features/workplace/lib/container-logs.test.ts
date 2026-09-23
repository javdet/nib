import { describe, expect, it } from 'vitest'
import type { ActionExecRun } from '@/features/dialogs/api/dialogs'
import { canViewContainerLogs } from './container-logs'

function run(overrides: Partial<ActionExecRun> = {}): ActionExecRun {
	return {
		status: 'running',
		startedAt: 1,
		attempt: 1,
		kind: 'container',
		...overrides,
	}
}

describe('canViewContainerLogs', () => {
	it('allows a running container run on the local executor', () => {
		expect(canViewContainerLogs(run(), 'local')).toBe(true)
	})

	it('refuses a remote executor', () => {
		expect(canViewContainerLogs(run(), 'remote')).toBe(false)
	})

	it('refuses a sub-agent run, which has no container', () => {
		expect(canViewContainerLogs(run({ kind: 'subagent' }), 'local')).toBe(false)
	})

	it('refuses a record from before container runs were told apart', () => {
		expect(canViewContainerLogs(run({ kind: undefined }), 'local')).toBe(false)
	})

	it('refuses a finished run', () => {
		expect(canViewContainerLogs(run({ status: 'done' }), 'local')).toBe(false)
	})

	it('refuses a row that has never run', () => {
		expect(canViewContainerLogs(undefined, 'local')).toBe(false)
	})

	it('refuses while the executor config is still loading', () => {
		expect(canViewContainerLogs(run(), null)).toBe(false)
	})
})
