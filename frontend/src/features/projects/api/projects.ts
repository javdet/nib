import { api } from '@/lib/api-client'

export interface Project {
	id: string
	name: string
	description: string
	createdAt: string
	updatedAt: string
}

export function listProjects(): Promise<Project[]> {
	return api.get<Project[]>('/projects')
}

export function getProject(id: string): Promise<Project> {
	return api.get<Project>(`/projects/${id}`)
}
