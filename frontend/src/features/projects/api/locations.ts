import { api } from '@/lib/api-client'

export interface Location {
	id: string
	projectId: string
	cloudId: string
	name: string
	description: string
	createdAt: string
	updatedAt: string
}

export function listLocations(
	projectId: string,
	cloudId: string,
): Promise<Location[]> {
	return api.get<Location[]>(
		`/projects/${projectId}/clouds/${cloudId}/locations`,
	)
}

export function getLocation(
	projectId: string,
	cloudId: string,
	locationId: string,
): Promise<Location> {
	return api.get<Location>(
		`/projects/${projectId}/clouds/${cloudId}/locations/${locationId}`,
	)
}
