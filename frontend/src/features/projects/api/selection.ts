import { api } from '@/lib/api-client'

export interface Selection {
	project: string
	environment: string
	cloud: string
	location: string
}

export function getSelection(): Promise<Selection> {
	return api.get<Selection>('/selection')
}

export function setSelection(selection: Selection): Promise<Selection> {
	return api.put<Selection>('/selection', selection)
}
