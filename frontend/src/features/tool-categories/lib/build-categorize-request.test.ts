import { describe, expect, it } from 'vitest'
import {
	CATEGORIZE_TOOLS_SKILL,
	buildCategorizeToolsRequest,
} from './build-categorize-request'
import type { CategorizedTool, ToolCategory } from '../api/tool-categories'

function category(name: string, description = ''): ToolCategory {
	return { name, description, patterns: [], toolCount: 0 }
}

function tool(name: string, server: string, description = ''): CategorizedTool {
	return { name, server, description }
}

describe('buildCategorizeToolsRequest', () => {
	const request = buildCategorizeToolsRequest(
		[category('cloud', 'Cloud provider APIs'), category('kubernetes')],
		[
			tool('list_pods', 'k8s-mcp', 'List pods in a namespace.'),
			tool('ec2_describe_instances', 'aws'),
		],
	)

	it('names the skill so the invocation does not depend on description matching', () => {
		expect(request).toContain(`name "${CATEGORIZE_TOOLS_SKILL}"`)
		expect(request).toContain('get_skill')
	})

	it('passes both lists the skill requires', () => {
		expect(request).toContain('categories:\n- cloud: Cloud provider APIs')
		expect(request).toContain('tools:\n- list_pods (k8s-mcp): List pods in a namespace.')
	})

	it('omits the separator when a category or tool has no description', () => {
		expect(request).toContain('- kubernetes\n')
		expect(request).toContain('- ec2_describe_instances (aws)')
		expect(request).not.toContain('- kubernetes:')
		expect(request).not.toContain('- ec2_describe_instances (aws):')
	})

	it('leads every tool line with the bare name, since that is what becomes the pattern', () => {
		for (const line of request.split('\n')) {
			if (!line.includes('(')) continue
			expect(line).toMatch(/^- [a-z0-9_]+ \(/)
		}
	})

	it('clips a long description the way the catalog tooltips clip it', () => {
		const long = `${'word '.repeat(120)}end.`
		const clipped = buildCategorizeToolsRequest(
			[category('cloud')],
			[tool('long_tool', 'srv', long)],
		)
		expect(clipped).toContain('…')
		expect(clipped.length).toBeLessThan(long.length)
	})

	it('renders empty lists without dangling entries', () => {
		const empty = buildCategorizeToolsRequest([], [])
		expect(empty).toContain('categories:\n\ntools:')
		expect(empty.trimEnd().endsWith('tools:')).toBe(true)
	})
})
