import { describe, expect, it } from 'vitest'
import { filterByQuery } from './filter-variables'

const entries = [
	{ name: 'CompanyName', description: 'Legal name', scope: 'global' },
	{
		name: 'Registry',
		description: 'Image registry host',
		scope: 'project',
		scopeName: 'billing',
	},
	{ name: 'Wiki', description: '', scope: 'global' },
]

describe('filterByQuery', () => {
	it('returns every entry for a blank query', () => {
		expect(filterByQuery(entries, '   ')).toEqual(entries)
	})

	it('matches on name, case-insensitively', () => {
		expect(filterByQuery(entries, 'company').map((e) => e.name)).toEqual([
			'CompanyName',
		])
	})

	it('matches on description', () => {
		expect(filterByQuery(entries, 'registry host').map((e) => e.name)).toEqual(
			['Registry'],
		)
	})

	it('matches on scope and scope name', () => {
		expect(filterByQuery(entries, 'billing').map((e) => e.name)).toEqual([
			'Registry',
		])
		expect(filterByQuery(entries, 'global').map((e) => e.name)).toEqual([
			'CompanyName',
			'Wiki',
		])
	})

	it('returns nothing when no entry matches', () => {
		expect(filterByQuery(entries, 'nope')).toEqual([])
	})
})
