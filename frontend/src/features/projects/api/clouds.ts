import { api } from '@/lib/api-client'

export interface Cloud {
	id: string
	projectId: string
	name: string
	description: string
	createdAt: string
	updatedAt: string
}

export function listClouds(
	projectId: string,
): Promise<Cloud[]> {
	return api.get<Cloud[]>(
		`/projects/${projectId}/clouds`,
	)
}

export function getCloud(
	projectId: string,
	cloudId: string,
): Promise<Cloud> {
	return api.get<Cloud>(
		`/projects/${projectId}/clouds/${cloudId}`,
	)
}
