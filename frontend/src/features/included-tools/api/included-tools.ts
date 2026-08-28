import { api } from '@/lib/api-client'

export interface SystemToolSummary {
	name: string
	description: string
	alwaysOn?: boolean
}

export interface IncludedTools {
	mode: string
	systemTools: SystemToolSummary[]
	includedTools: string[]
}

export interface CatalogTool {
	server: string
	name: string
	description: string
}

export function getIncludedTools(mode: string): Promise<IncludedTools> {
	return api.get<IncludedTools>(
		`/included-tools/${encodeURIComponent(mode)}`,
	)
}

export function setIncludedTools(
	mode: string,
	tools: string[],
): Promise<IncludedTools> {
	return api.put<IncludedTools>(
		`/included-tools/${encodeURIComponent(mode)}`,
		{ tools },
	)
}

export function listCatalogTools(): Promise<CatalogTool[]> {
	return api.get<CatalogTool[]>('/mcp/catalog-tools')
}
