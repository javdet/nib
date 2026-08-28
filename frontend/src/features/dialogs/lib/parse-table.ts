import type { TableData } from '@/features/dialogs/api/dialogs'

const CREATE_TABLE_TOOL = 'create_table'

function normalizeRow(cells: unknown[], colCount: number): string[] {
	const row = new Array<string>(colCount).fill('')
	for (let j = 0; j < colCount; j++) {
		if (j < cells.length && cells[j] != null) {
			row[j] = String(cells[j])
		}
	}
	return row
}

export function parseCreateTableArguments(
	argumentsJSON: string,
): TableData | null {
	try {
		const args = JSON.parse(argumentsJSON) as {
			title?: unknown
			columns?: unknown
			rows?: unknown
		}
		if (!Array.isArray(args.columns) || args.columns.length === 0) {
			return null
		}

		const columns: string[] = []
		for (const col of args.columns) {
			if (typeof col !== 'string' || col.trim() === '') {
				return null
			}
			columns.push(col.trim())
		}

		const title =
			typeof args.title === 'string' && args.title.trim() !== ''
				? args.title.trim()
				: undefined

		if (!Array.isArray(args.rows)) {
			return null
		}

		const rows: string[][] = []
		for (const rowRaw of args.rows) {
			if (!Array.isArray(rowRaw)) {
				return null
			}
			rows.push(normalizeRow(rowRaw, columns.length))
		}

		return { title, columns, rows }
	} catch {
		return null
	}
}

export function parseCreateTableFromToolCalls(
	toolCalls?: { function?: { name?: string; arguments?: string } }[],
): TableData[] {
	if (!toolCalls?.length) {
		return []
	}

	const tables: TableData[] = []
	for (const tc of toolCalls) {
		if (tc.function?.name !== CREATE_TABLE_TOOL) {
			continue
		}
		const args = tc.function.arguments
		if (!args) {
			continue
		}
		const table = parseCreateTableArguments(args)
		if (table) {
			tables.push(table)
		}
	}
	return tables
}
