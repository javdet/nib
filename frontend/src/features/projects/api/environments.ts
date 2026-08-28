import { api } from '@/lib/api-client'

export interface Environment {
	id: string
	projectId: string
	name: string
	description: string
	createdAt: string
	updatedAt: string
}

export function listEnvironments(
	projectId: string,
): Promise<Environment[]> {
	return api.get<Environment[]>(
		`/projects/${projectId}/environments`,
	)
}

export function getEnvironment(
	projectId: string,
	envId: string,
): Promise<Environment> {
	return api.get<Environment>(
		`/projects/${projectId}/environments/${envId}`,
	)
}
