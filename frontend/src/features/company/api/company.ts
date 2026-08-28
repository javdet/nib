import { api } from '@/lib/api-client'

export interface CompanyInfo {
	companyName: string
	companyDescription: string
	versionControlSystem: string
	gitBaseUrl: string
	gitUsername: string
	gitEmail: string
	ciCdSystem: string
	taskTracker: string
	wiki: string
	messenger: string
}

export function getCompany() {
	return api.get<CompanyInfo>('/company')
}

export function updateCompany(data: CompanyInfo) {
	return api.put<CompanyInfo>('/company', data)
}
