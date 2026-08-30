/**
 * Removes // line comments and block comments from JSONC content, leaving
 * comment-like sequences inside string literals alone — a mirror of the
 * backend's stripJSONComments, so the editor accepts exactly the files the
 * backend can read (a "//" in a URL is not a comment).
 */
export function stripJSONComments(content: string): string {
	let out = ''
	let inString = false
	let escaped = false

	for (let i = 0; i < content.length; i++) {
		const ch = content[i]

		if (inString) {
			out += ch
			if (escaped) {
				escaped = false
			} else if (ch === '\\') {
				escaped = true
			} else if (ch === '"') {
				inString = false
			}
			continue
		}

		if (ch === '"') {
			inString = true
			out += ch
			continue
		}

		if (ch === '/' && content[i + 1] === '/') {
			i += 2
			while (i < content.length && content[i] !== '\n') i++
			if (i < content.length) out += '\n'
			continue
		}

		if (ch === '/' && content[i + 1] === '*') {
			i += 2
			while (
				i + 1 < content.length &&
				!(content[i] === '*' && content[i + 1] === '/')
			) {
				i++
			}
			i++
			continue
		}

		out += ch
	}

	return out
}

/**
 * Reports why mcp.json cannot be saved, or null when it can. Only structure the
 * backend has to be able to read back is checked: a server that is unreachable,
 * or a ${SECRET} that has no value yet, still saves — that shows up when the
 * server's tools are listed.
 */
export function validateMCPJSON(text: string): string | null {
	const trimmed = stripJSONComments(text).trim()
	if (!trimmed) {
		return 'JSON cannot be empty'
	}
	let parsed: unknown
	try {
		parsed = JSON.parse(trimmed)
	} catch (err) {
		return err instanceof Error
			? `Invalid JSON syntax: ${err.message}`
			: 'Invalid JSON syntax'
	}
	if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
		return 'Root value must be a JSON object'
	}
	const doc = parsed as Record<string, unknown>
	if (!('mcpServers' in doc)) {
		return 'Missing required "mcpServers" property'
	}
	const servers = doc.mcpServers
	if (
		servers === null ||
		typeof servers !== 'object' ||
		Array.isArray(servers)
	) {
		return '"mcpServers" must be an object'
	}
	for (const [name, entry] of Object.entries(servers)) {
		if (entry === null || typeof entry !== 'object' || Array.isArray(entry)) {
			return `"${name}" must be an object`
		}
	}
	return null
}
