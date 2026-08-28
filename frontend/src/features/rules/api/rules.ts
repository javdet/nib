import { api } from '@/lib/api-client'

export interface Rule {
	name: string
	content: string
}

export interface RuleList {
	rules: string[]
}

export function listRules(): Promise<RuleList> {
	return api.get<RuleList>('/rules')
}

export function getRule(name: string): Promise<Rule> {
	return api.get<Rule>(`/rules/${encodeURIComponent(name)}`)
}

export function createRule(name: string, content = ''): Promise<Rule> {
	return api.post<Rule>('/rules', { name, content })
}

export function updateRule(name: string, content: string): Promise<void> {
	return api.put<void>(`/rules/${encodeURIComponent(name)}`, { content })
}

export function deleteRule(name: string): Promise<void> {
	return api.delete<void>(`/rules/${encodeURIComponent(name)}`)
}
