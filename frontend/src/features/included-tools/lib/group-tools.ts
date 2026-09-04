export type ToolEntry = {
	name: string
	server: string
	description: string
}

export const unknownServer = ''

export function formatServerLabel(server: string): string {
	return server || 'Unknown'
}

export function groupByServer(tools: ToolEntry[]): Map<string, ToolEntry[]> {
	const groups = new Map<string, ToolEntry[]>()
	for (const tool of tools) {
		const list = groups.get(tool.server) ?? []
		list.push(tool)
		groups.set(tool.server, list)
	}
	for (const [, list] of groups) {
		list.sort((a, b) => a.name.localeCompare(b.name))
	}
	return new Map([...groups.entries()].sort(([a], [b]) => {
		if (a === unknownServer) {
			return 1
		}
		if (b === unknownServer) {
			return -1
		}
		return a.localeCompare(b)
	}))
}

export function matchesFilter(tool: ToolEntry, filter: string): boolean {
	if (!filter) {
		return true
	}
	const q = filter.toLowerCase()
	return (
		tool.name.toLowerCase().includes(q) ||
		tool.server.toLowerCase().includes(q) ||
		tool.description.toLowerCase().includes(q)
	)
}
