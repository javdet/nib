import { api } from '@/lib/api-client'

export interface ToolCategory {
	name: string
	description: string
	patterns: string[]
	toolCount: number
}

export interface CategorizedTool {
	server: string
	name: string
	description: string
	categories?: string[] | null
}

export function listToolCategories(): Promise<ToolCategory[]> {
	return api.get<ToolCategory[]>('/tool-categories')
}

export function setCategoryPatterns(
	name: string,
	patterns: string[],
): Promise<ToolCategory> {
	return api.put<ToolCategory>(
		`/tool-categories/${encodeURIComponent(name)}/patterns`,
		{ patterns },
	)
}

export function listCategoryTools(name: string): Promise<CategorizedTool[]> {
	return api.get<CategorizedTool[]>(
		`/tool-categories/${encodeURIComponent(name)}/tools`,
	)
}

export function listUncategorizedTools(): Promise<CategorizedTool[]> {
	return api.get<CategorizedTool[]>('/tool-categories/uncategorized/tools')
}

export function patternsToLines(patterns: string[]): string {
	return patterns.join('\n')
}

export function linesToPatterns(lines: string): string[] {
	return lines
		.split('\n')
		.map((line) => line.trim())
		.filter((line) => line !== '')
}
