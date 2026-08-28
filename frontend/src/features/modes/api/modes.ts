import { api } from '@/lib/api-client'

export function listModes(): Promise<string[]> {
	return api.get<string[]>('/modes')
}
