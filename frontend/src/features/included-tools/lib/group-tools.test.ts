import { describe, expect, it } from 'vitest'
import {
	formatServerLabel,
	groupByServer,
	matchesFilter,
	type ToolEntry,
	unknownServer,
} from './group-tools'

function tool(
	name: string,
	server: string,
	description = '',
): ToolEntry {
	return { name, server, description }
}

describe('groupByServer', () => {
	it('sorts named servers alphabetically with unknown last', () => {
		const grouped = groupByServer([
			tool('zeta', 'zebra'),
			tool('alpha', 'alpha'),
			tool('orphan', unknownServer),
			tool('beta', 'beta'),
		])

		expect([...grouped.keys()]).toEqual(['alpha', 'beta', 'zebra', unknownServer])
	})

	it('sorts tools within each server by name', () => {
		const grouped = groupByServer([
			tool('charlie', 'github'),
			tool('alpha', 'github'),
			tool('bravo', 'github'),
		])

		expect(grouped.get('github')?.map((t) => t.name)).toEqual([
			'alpha',
			'bravo',
			'charlie',
		])
	})

	it('groups empty-server tools under unknownServer', () => {
		const grouped = groupByServer([
			tool('one', unknownServer),
			tool('two', unknownServer),
		])

		expect(grouped.get(unknownServer)?.map((t) => t.name)).toEqual([
			'one',
			'two',
		])
	})
})

describe('matchesFilter', () => {
	it('matches all tools when filter is empty', () => {
		expect(matchesFilter(tool('foo', 'github', 'desc'), '')).toBe(true)
	})

	it('matches by name, server, or description case-insensitively', () => {
		const entry = tool('actions_get', 'GitHub', 'List workflow runs')
		expect(matchesFilter(entry, 'actions')).toBe(true)
		expect(matchesFilter(entry, 'github')).toBe(true)
		expect(matchesFilter(entry, 'WORKFLOW')).toBe(true)
		expect(matchesFilter(entry, 'missing')).toBe(false)
	})
})

describe('formatServerLabel', () => {
	it('returns Unknown for empty server', () => {
		expect(formatServerLabel(unknownServer)).toBe('Unknown')
	})

	it('returns the server name when present', () => {
		expect(formatServerLabel('github')).toBe('github')
	})
})
