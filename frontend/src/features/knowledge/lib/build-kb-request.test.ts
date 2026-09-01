import { describe, expect, it } from 'vitest'
import {
	BUILD_KNOWLEDGE_BASE_SKILL,
	buildKnowledgeBaseRequest,
	parseRepositoryList,
} from './build-kb-request'

describe('parseRepositoryList', () => {
	it.each([
		['', []],
		['   ', []],
		[',,', []],
		['owner/repo', ['owner/repo']],
		['  owner/repo  ', ['owner/repo']],
		['a/one, a/two', ['a/one', 'a/two']],
		['a/one,a/two,', ['a/one', 'a/two']],
		['a/one, , a/two', ['a/one', 'a/two']],
		['a/one, a/one', ['a/one']],
		[
			'acme/shop2-infra, https://github.com/acme/helm-charts',
			['acme/shop2-infra', 'https://github.com/acme/helm-charts'],
		],
	])('%s -> %j', (raw, expected) => {
		expect(parseRepositoryList(raw)).toEqual(expected)
	})

	it('keeps the order the operator typed', () => {
		expect(parseRepositoryList('c/three, a/one, b/two')).toEqual([
			'c/three',
			'a/one',
			'b/two',
		])
	})
})

describe('buildKnowledgeBaseRequest', () => {
	const request = buildKnowledgeBaseRequest('shop', [
		'acme/shop2-infra',
		'https://github.com/acme/helm-charts',
	])

	it('names the skill so the invocation does not depend on description matching', () => {
		expect(request).toContain(`name "${BUILD_KNOWLEDGE_BASE_SKILL}"`)
		expect(request).toContain('get_skill')
	})

	it('passes both parameters the skill requires', () => {
		expect(request).toContain('project: shop')
		expect(request).toContain(
			'repositories: acme/shop2-infra, https://github.com/acme/helm-charts',
		)
	})

	it('renders a single repository without a trailing separator', () => {
		expect(buildKnowledgeBaseRequest('default', ['a/one'])).toContain(
			'repositories: a/one',
		)
	})
})
