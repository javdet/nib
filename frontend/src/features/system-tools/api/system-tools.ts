import { api } from '@/lib/api-client'

export interface SystemTool {
	name: string
	description: string
	parameters: unknown
	modes: string[]
	always_on?: boolean
}

export function listSystemTools(): Promise<SystemTool[]> {
	return api.get<SystemTool[]>('/system/tools')
}
