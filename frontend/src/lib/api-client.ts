const BASE_URL = import.meta.env.VITE_API_URL ?? '/api/v1'

export function apiUrl(path: string): string {
	return `${BASE_URL}${path}`
}

export class ApiError extends Error {
	status: number

	constructor(status: number, message: string) {
		super(message)
		this.name = 'ApiError'
		this.status = status
	}
}

async function parseErrorBody(res: Response): Promise<string> {
	try {
		const body = await res.json()
		if (body && typeof body.error === 'string') {
			return body.error
		}
	} catch {
		// response wasn't JSON — fall through to statusText
	}
	return res.statusText
}

async function request<T>(path: string, options?: RequestInit): Promise<T> {
	const hasBody = options?.body != null
	const url = `${BASE_URL}${path}`
	const res = await fetch(url, {
		headers: hasBody
			? {
					'Content-Type': 'application/json',
					...options?.headers,
				}
			: options?.headers,
		...options,
	})

	if (!res.ok) {
		const message = await parseErrorBody(res)
		throw new ApiError(res.status, message)
	}

	if (res.status === 204) {
		return undefined as T
	}

	return res.json() as Promise<T>
}

export interface PagedResponse<T> {
	items: T
	total: number
}

async function requestWithHeaders<T>(
	path: string,
	options?: RequestInit,
): Promise<{ data: T; headers: Headers }> {
	const url = `${BASE_URL}${path}`
	const res = await fetch(url, options)

	if (!res.ok) {
		const message = await parseErrorBody(res)
		throw new ApiError(res.status, message)
	}

	if (res.status === 204) {
		return { data: undefined as T, headers: res.headers }
	}

	const data = (await res.json()) as T
	return { data, headers: res.headers }
}

export const api = {
	get: <T>(path: string, signal?: AbortSignal) =>
		request<T>(path, { signal }),

	getWithTotal: async <T>(path: string, signal?: AbortSignal) => {
		const { data, headers } = await requestWithHeaders<T>(path, { signal })
		const totalHeader = headers.get('X-Total-Count')
		const total = totalHeader ? Number.parseInt(totalHeader, 10) : 0
		return { data, total: Number.isNaN(total) ? 0 : total }
	},

	post: <T>(path: string, body: unknown, signal?: AbortSignal) =>
		request<T>(path, {
			method: 'POST',
			body: JSON.stringify(body),
			signal,
		}),

	put: <T>(path: string, body: unknown) =>
		request<T>(path, {
			method: 'PUT',
			body: JSON.stringify(body),
		}),

	delete: <T>(path: string) =>
		request<T>(path, { method: 'DELETE' }),

	upload: async <T>(path: string, formData: FormData, signal?: AbortSignal) => {
		const url = `${BASE_URL}${path}`
		const res = await fetch(url, {
			method: 'POST',
			body: formData,
			signal,
		})
		if (!res.ok) {
			const message = await parseErrorBody(res)
			throw new ApiError(res.status, message)
		}
		if (res.status === 204) {
			return undefined as T
		}
		return res.json() as Promise<T>
	},
}

/** Statuses the dev proxy reports when the upstream connection dies mid-request. */
const TRANSPORT_ERROR_STATUSES = new Set([502, 503, 504])

/**
 * Reports whether a request failed because the connection broke rather than
 * because the backend rejected it. Such a failure says nothing about the work
 * the backend started, so callers may recover by re-reading server state.
 */
export function isTransportError(err: unknown): boolean {
	if (err instanceof ApiError) {
		return TRANSPORT_ERROR_STATUSES.has(err.status)
	}
	return err instanceof TypeError
}

export function extractErrorMessage(err: unknown): string {
	if (err instanceof ApiError) {
		return err.message
	}
	if (err instanceof Error) {
		return err.message
	}
	return 'An unexpected error occurred'
}
