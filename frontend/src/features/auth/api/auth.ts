import { api } from '@/lib/api-client'

export interface SessionStatus {
	/** False only when the backend runs with NIB_INSECURE_NO_AUTH. */
	required: boolean
	authenticated: boolean
}

export function getSession(): Promise<SessionStatus> {
	return api.get<SessionStatus>('/auth/session')
}

/** Exchanges the API token for an HttpOnly session cookie. */
export function login(token: string): Promise<void> {
	return api.post<void>('/auth/session', { token })
}

export function logout(): Promise<void> {
	return api.delete<void>('/auth/session')
}
