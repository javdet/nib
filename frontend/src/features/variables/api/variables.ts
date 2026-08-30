import { api } from '@/lib/api-client'

export type VariableKind = 'string' | 'list'

export interface Variable {
	id: string
	scope: string
	scopeName?: string
	name: string
	description: string
	value: string
	kind: VariableKind
	deletable: boolean
	createdAt: string
	updatedAt: string
}

export interface VariableInput {
	scope?: string
	scopeName?: string
	name: string
	description?: string
	value: string
	kind?: VariableKind
}

export function listVariables(): Promise<Variable[]> {
	return api.get<Variable[]>('/variables')
}

export function createVariable(input: VariableInput): Promise<Variable> {
	return api.post<Variable>('/variables', input)
}

export function updateVariable(
	id: string,
	input: VariableInput,
): Promise<Variable> {
	return api.put<Variable>(`/variables/${encodeURIComponent(id)}`, input)
}

export function deleteVariable(id: string): Promise<void> {
	return api.delete<void>(`/variables/${encodeURIComponent(id)}`)
}

export function listValueToLines(value: string): string {
	try {
		const items = JSON.parse(value) as unknown
		if (!Array.isArray(items)) {
			return value
		}
		return items.map((item) => String(item)).join('\n')
	} catch {
		return value
	}
}

export function linesToListValue(lines: string): string {
	const items = lines
		.split('\n')
		.map((line) => line.trim())
		.filter((line) => line !== '')
	return JSON.stringify(items)
}
