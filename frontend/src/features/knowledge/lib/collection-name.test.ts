import { describe, expect, it } from 'vitest'
import {
	DEFAULT_COLLECTION,
	collectionForProject,
	isValidKnowledgeCollectionName,
} from './collection-name'

describe('isValidKnowledgeCollectionName', () => {
	it.each([
		['default', true],
		['myproject', true],
		['kiss2.prod', true],
		['infra-docs', true],
		['infra_docs', true],
		['', false],
		['   ', false],
		['.hidden', false],
		['-leading-dash', false],
		['has space', false],
		['has/slash', false],
		['..', false],
		['a'.repeat(65), false],
		['a'.repeat(64), true],
	])('%s -> %s', (name, expected) => {
		expect(isValidKnowledgeCollectionName(name)).toBe(expected)
	})
})

describe('collectionForProject', () => {
	it('falls back to the default when nothing is selected', () => {
		expect(collectionForProject(null)).toBe(DEFAULT_COLLECTION)
		expect(collectionForProject(undefined)).toBe(DEFAULT_COLLECTION)
		expect(collectionForProject('')).toBe(DEFAULT_COLLECTION)
	})

	it('treats the "any" sentinel as no project', () => {
		expect(collectionForProject('any')).toBe(DEFAULT_COLLECTION)
	})

	it('uses the project name when it is a usable collection name', () => {
		expect(collectionForProject('myproject')).toBe('myproject')
		expect(collectionForProject('  myproject  ')).toBe('myproject')
	})

	it('falls back when the project name is not a usable collection name', () => {
		expect(collectionForProject('My Project')).toBe(DEFAULT_COLLECTION)
		expect(collectionForProject('../escape')).toBe(DEFAULT_COLLECTION)
	})
})
