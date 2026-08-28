import { api } from '@/lib/api-client'

export interface VersionInfo {
	version: string
}

export function getVersion(): Promise<VersionInfo> {
	return api.get<VersionInfo>('/version')
}
