import { api } from '@/lib/api-client'

export interface Secret {
	id: string
	scope: string
	name: string
	description: string
	createdAt: string
	updatedAt: string
}

export interface SecretInput {
	scope?: string
	name: string
	description?: string
	value?: string
}

export function listSecrets(): Promise<Secret[]> {
	return api.get<Secret[]>('/secrets')
}

export function createSecret(input: SecretInput): Promise<Secret> {
	return api.post<Secret>('/secrets', input)
}

export function updateSecret(
	id: string,
	input: SecretInput,
): Promise<Secret> {
	return api.put<Secret>(`/secrets/${encodeURIComponent(id)}`, input)
}

export function deleteSecret(id: string): Promise<void> {
	return api.delete<void>(`/secrets/${encodeURIComponent(id)}`)
}
