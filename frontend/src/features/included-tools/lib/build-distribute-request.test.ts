import { describe, expect, it } from 'vitest'
import {
	DISTRIBUTE_TOOLS_SKILL,
	buildDistributeToolsRequest,
} from './build-distribute-request'
import type { CatalogTool } from '../api/included-tools'

function tool(name: string, server: string, description = ''): CatalogTool {
	return { name, server, description }
}

describe('buildDistributeToolsRequest', () => {
	const request = buildDistributeToolsRequest(
		['main', 'execute', 'incident'],
		[
			tool('k8s_delete_pod', 'k8s-mcp', 'Delete a pod in a namespace.'),
			tool('github_merge_pull_request', 'github'),
		],
	)

	it('names the skill so the invocation does not depend on description matching', () => {
		expect(request).toContain(`name "${DISTRIBUTE_TOOLS_SKILL}"`)
		expect(request).toContain('get_skill')
	})

	it('passes both lists the skill requires', () => {
		expect(request).toContain('modes:\n- main\n- execute\n- incident')
		expect(request).toContain(
			'tools:\n- k8s_delete_pod (k8s-mcp): Delete a pod in a namespace.',
		)
	})

	it('omits the separator when a tool has no description', () => {
		expect(request).toContain('- github_merge_pull_request (github)')
		expect(request).not.toContain('- github_merge_pull_request (github):')
	})

	it('leads every tool line with the bare name, since that is what the tool takes', () => {
		for (const line of request.split('\n')) {
			if (!line.includes('(')) continue
			expect(line).toMatch(/^- [a-z0-9_]+ \(/)
		}
	})

	it('carries no policy of its own - the skill owns the per-mode rules', () => {
		expect(request.toLowerCase()).not.toContain('remove')
	})

	it('clips a long description the way the catalog tooltips clip it', () => {
		const long = `${'word '.repeat(120)}end.`
		const clipped = buildDistributeToolsRequest(
			['main'],
			[tool('long_tool', 'srv', long)],
		)
		expect(clipped).toContain('…')
		expect(clipped.length).toBeLessThan(long.length)
	})

	it('renders empty lists without dangling entries', () => {
		const empty = buildDistributeToolsRequest([], [])
		expect(empty).toContain('modes:\n\ntools:')
		expect(empty.trimEnd().endsWith('tools:')).toBe(true)
	})
})
