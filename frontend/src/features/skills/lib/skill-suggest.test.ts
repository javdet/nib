import { describe, expect, it } from 'vitest'
import type { SkillMeta } from '../api/skills'
import {
	MAX_SKILL_SUGGESTIONS,
	applySkillSelection,
	matchSkills,
	parseSkillQuery,
} from './skill-suggest'

function skill(name: string, description = ''): SkillMeta {
	return { name, description, category: 'searchable' }
}

describe('parseSkillQuery', () => {
	it.each([
		['/', ''],
		['/dep', 'dep'],
		['/DEP', 'DEP'],
		['/build-knowledge-base', 'build-knowledge-base'],
		['/a.b_c-1', 'a.b_c-1'],
		['', null],
		['hello', null],
		['/dep ', null],
		['/dep loy', null],
		['/a/b', null],
		['	/dep', null],
		['/dep\n', null],
		['hi /dep', null],
	])('%j -> %j', (draft, expected) => {
		expect(parseSkillQuery(draft)).toBe(expected)
	})
})

describe('matchSkills', () => {
	const skills = [
		skill('jira-todo-tasks', 'Retrieve todo tasks from Jira.'),
		skill('build-knowledge-base', 'Build or refresh the knowledge base.'),
		skill('jira-get-board', 'Show the board as a table.'),
		skill('deploy-service', 'Roll out a service to an environment.'),
	]

	it('lists everything alphabetically for an empty query', () => {
		expect(matchSkills(skills, '').map((s) => s.name)).toEqual([
			'build-knowledge-base',
			'deploy-service',
			'jira-get-board',
			'jira-todo-tasks',
		])
	})

	it('ranks a name prefix above a word prefix above a substring', () => {
		const ranked = matchSkills(
			[
				skill('a-board-tool', ''),
				skill('reboard', ''),
				skill('board-viewer', ''),
			],
			'board',
		).map((s) => s.name)
		expect(ranked).toEqual(['board-viewer', 'a-board-tool', 'reboard'])
	})

	it('keeps equal matches alphabetical', () => {
		expect(matchSkills(skills, 'jira').map((s) => s.name)).toEqual([
			'jira-get-board',
			'jira-todo-tasks',
		])
	})

	it('ranks a description-only hit below every name hit', () => {
		const ranked = matchSkills(
			[
				skill('mentions-board', 'Unrelated.'),
				skill('summary-tool', 'Show the board as a table.'),
			],
			'board',
		).map((s) => s.name)
		expect(ranked).toEqual(['mentions-board', 'summary-tool'])
	})

	it('matches the description when no name does', () => {
		expect(matchSkills(skills, 'refresh').map((s) => s.name)).toEqual([
			'build-knowledge-base',
		])
	})

	it('is case insensitive on both sides', () => {
		expect(matchSkills([skill('Deploy-Service', '')], 'deploy')).toHaveLength(1)
		expect(matchSkills(skills, 'JIRA')).toHaveLength(2)
	})

	it('returns nothing when nothing matches', () => {
		expect(matchSkills(skills, 'zzz')).toEqual([])
	})

	it('honours the limit', () => {
		expect(matchSkills(skills, '', 2)).toHaveLength(2)
		expect(matchSkills(skills, 'jira', 1).map((s) => s.name)).toEqual([
			'jira-get-board',
		])
	})

	it('caps at MAX_SKILL_SUGGESTIONS by default', () => {
		const many = Array.from({ length: MAX_SKILL_SUGGESTIONS + 10 }, (_, i) =>
			skill(`skill-${String(i).padStart(3, '0')}`),
		)
		expect(matchSkills(many, '')).toHaveLength(MAX_SKILL_SUGGESTIONS)
	})
})

describe('applySkillSelection', () => {
	it('inserts the command with a trailing space for the argument', () => {
		expect(applySkillSelection('jira-get-board')).toBe('/jira-get-board ')
	})
})
