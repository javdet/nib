import { api } from '@/lib/api-client'
import type { SkillCategory } from '@/lib/frontmatter'

export interface Skill {
	name: string
	content: string
}

export interface SkillMeta {
	name: string
	description: string
	category: SkillCategory
}

export interface SkillList {
	skills: SkillMeta[]
}

export function listSkills(): Promise<SkillList> {
	return api.get<SkillList>('/skills')
}

export function getSkill(name: string): Promise<Skill> {
	return api.get<Skill>(`/skills/${encodeURIComponent(name)}`)
}

export function createSkill(name: string, content = ''): Promise<Skill> {
	return api.post<Skill>('/skills', { name, content })
}

export function updateSkill(name: string, content: string): Promise<void> {
	return api.put<void>(`/skills/${encodeURIComponent(name)}`, { content })
}

export function deleteSkill(name: string): Promise<void> {
	return api.delete<void>(`/skills/${encodeURIComponent(name)}`)
}
