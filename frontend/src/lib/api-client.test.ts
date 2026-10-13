import { afterEach, describe, expect, it, vi } from 'vitest'
import { api, ApiError, isTransportError, setUnauthorizedHandler } from './api-client'

function stubFetch(
	status: number,
	body: unknown = {},
	headers: Record<string, string> = {},
) {
	const fetchMock = vi.fn<typeof fetch>(async () =>
		new Response(status === 204 ? null : JSON.stringify(body), {
			status,
			headers,
		}),
	)
	vi.stubGlobal('fetch', fetchMock)
	return fetchMock
}

afterEach(() => {
	vi.unstubAllGlobals()
	setUnauthorizedHandler(null)
})

describe('api client', () => {
	it('declares a JSON body as application/json', async () => {
		const fetchMock = stubFetch(200)
		await api.put('/rules/x', { content: 'y' })
		const init = fetchMock.mock.calls[0]?.[1]
		expect(init?.headers).toMatchObject({ 'Content-Type': 'application/json' })
	})

	it('reports a 401 to the registered handler', async () => {
		stubFetch(401, { error: 'unauthorized' })
		const onUnauthorized = vi.fn()
		setUnauthorizedHandler(onUnauthorized)

		await expect(api.get('/rules')).rejects.toBeInstanceOf(ApiError)
		await expect(api.getWithTotal('/dialogs')).rejects.toBeInstanceOf(ApiError)
		await expect(api.upload('/knowledge/documents', new FormData())).rejects.toBeInstanceOf(ApiError)
		expect(onUnauthorized).toHaveBeenCalledTimes(3)
	})

	it('leaves other failures alone', async () => {
		stubFetch(403, { error: 'cross-origin request refused' })
		const onUnauthorized = vi.fn()
		setUnauthorizedHandler(onUnauthorized)

		await expect(api.post('/rules', {})).rejects.toMatchObject({
			status: 403,
			message: 'cross-origin request refused',
		})
		expect(onUnauthorized).not.toHaveBeenCalled()
	})

	it('treats a 502 the backend wrote as a final answer', async () => {
		stubFetch(502, { error: 'The AI provider rejected the API key.' }, { 'X-Nib-Error': '1' })
		const err = await api.post('/dialogs/x/messages', {}).catch((e: unknown) => e)
		expect(err).toMatchObject({ status: 502, fromBackend: true })
		expect(isTransportError(err)).toBe(false)
	})

	it('treats a proxy 502 as a dropped connection', async () => {
		stubFetch(502, { error: 'proxy error: socket hang up' })
		const err = await api.post('/dialogs/x/messages', {}).catch((e: unknown) => e)
		expect(isTransportError(err)).toBe(true)
	})
})
