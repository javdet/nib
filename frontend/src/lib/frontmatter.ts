export type SkillCategory = 'searchable' | 'included'

export interface FrontmatterFields {
	name: string
	description: string
	body: string
	category?: SkillCategory
}

const FRONTMATTER_KEYS = ['name', 'description'] as const

function needsQuoting(value: string): boolean {
	return (
		value === '' ||
		value.includes(':') ||
		value.includes('#') ||
		value.includes('\n') ||
		value.includes('"') ||
		value.includes("'") ||
		/^\s|\s$/.test(value) ||
		/^[&*!|>@[\]{},`]/.test(value)
	)
}

function quoteYamlValue(value: string): string {
	if (!needsQuoting(value)) {
		return value
	}
	return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

function unquoteYamlValue(value: string): string {
	const trimmed = value.trim()
	if (
		(trimmed.startsWith('"') && trimmed.endsWith('"')) ||
		(trimmed.startsWith("'") && trimmed.endsWith("'"))
	) {
		const inner = trimmed.slice(1, -1)
		if (trimmed.startsWith('"')) {
			return inner.replace(/\\"/g, '"').replace(/\\\\/g, '\\')
		}
		return inner
	}
	return trimmed
}

function parseFrontmatterBlock(
	block: string,
): Pick<FrontmatterFields, 'name' | 'description' | 'category'> {
	const result: Pick<FrontmatterFields, 'name' | 'description' | 'category'> = {
		name: '',
		description: '',
	}
	for (const line of block.split('\n')) {
		const trimmed = line.trim()
		if (!trimmed || trimmed.startsWith('#')) continue
		const colon = trimmed.indexOf(':')
		if (colon < 0) continue
		const key = trimmed.slice(0, colon).trim()
		const value = unquoteYamlValue(trimmed.slice(colon + 1))
		if (key === 'name') result.name = value
		if (key === 'description') result.description = value
		if (key === 'category') {
			const normalized = value.trim().toLowerCase()
			if (normalized === 'included') result.category = 'included'
			else if (normalized === 'searchable') result.category = 'searchable'
		}
	}
	return result
}

/**
 * Parses markdown content with optional YAML frontmatter into separate fields.
 * If no frontmatter is present, name and description are empty and body is the full content.
 */
export function parseFrontmatter(content: string): FrontmatterFields {
	const normalized = content.replace(/^\uFEFF/, '')
	if (!normalized.startsWith('---')) {
		return { name: '', description: '', body: content }
	}

	const afterOpen = normalized.startsWith('---\r\n')
		? normalized.slice(5)
		: normalized.startsWith('---\n')
			? normalized.slice(4)
			: null

	if (afterOpen === null) {
		return { name: '', description: '', body: content }
	}

	const closes = ['\n---\n', '\n---\r\n', '\r\n---\n', '\r\n---\r\n']
	let best = -1
	let bestEnd = 0
	for (const sep of closes) {
		const i = afterOpen.indexOf(sep)
		if (i >= 0 && (best < 0 || i < best)) {
			best = i
			bestEnd = i + sep.length
		}
	}

	if (best < 0) {
		for (const suff of ['\n---', '\r\n---']) {
			if (afterOpen.endsWith(suff)) {
				const block = afterOpen.slice(0, -suff.length)
				const fields = parseFrontmatterBlock(block)
				return { ...fields, body: '' }
			}
		}
		return { name: '', description: '', body: content }
	}

	const block = afterOpen.slice(0, best)
	const body = afterOpen.slice(bestEnd)
	const fields = parseFrontmatterBlock(block)
	return { ...fields, body }
}

/**
 * Builds markdown content from frontmatter fields.
 * When body is empty, emits frontmatter only (no trailing blank line).
 */
export function buildContent(fields: FrontmatterFields): string {
	const lines = ['---']
	for (const key of FRONTMATTER_KEYS) {
		lines.push(`${key}: ${quoteYamlValue(fields[key])}`)
	}
	if (fields.category === 'included') {
		lines.push('category: included')
	}
	lines.push('---')

	if (fields.body) {
		const body = fields.body.replace(/^\n+/, '')
		return `${lines.join('\n')}\n\n${body}`
	}

	return lines.join('\n')
}
