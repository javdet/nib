import { useCallback, useEffect, useMemo, useState, type ReactNode } from 'react'
import { Button } from '@/components/ui/button'
import { extractErrorMessage, setUnauthorizedHandler } from '@/lib/api-client'
import { getSession, logout } from '../api/auth'
import { AuthContext } from '../auth-context'
import { LoginPage } from '../pages/login-page'

type GateState =
	| { status: 'loading' }
	| { status: 'error'; message: string }
	| { status: 'ready'; required: boolean; authenticated: boolean }

/**
 * Renders the app only once the backend accepts this browser. Nothing under
 * it mounts before that, so no page fires a request that would just 401.
 */
export function AuthGate({ children }: { children: ReactNode }) {
	const [state, setState] = useState<GateState>({ status: 'loading' })

	const refresh = useCallback(async () => {
		try {
			const session = await getSession()
			setState({ status: 'ready', ...session })
		} catch (err) {
			setState({ status: 'error', message: extractErrorMessage(err) })
		}
	}, [])

	useEffect(() => {
		void refresh()
	}, [refresh])

	// A 401 from any later request means the session expired or the token was
	// rotated underneath us.
	useEffect(() => {
		setUnauthorizedHandler(() => {
			setState((prev) =>
				prev.status === 'ready' && prev.required
					? { ...prev, authenticated: false }
					: prev,
			)
		})
		return () => setUnauthorizedHandler(null)
	}, [])

	const handleSignOut = useCallback(async () => {
		try {
			await logout()
		} finally {
			setState((prev) =>
				prev.status === 'ready' ? { ...prev, authenticated: false } : prev,
			)
		}
	}, [])

	const required = state.status === 'ready' && state.required
	const contextValue = useMemo(
		() => ({ required, signOut: handleSignOut }),
		[required, handleSignOut],
	)

	if (state.status === 'loading') {
		return (
			<div className="flex h-screen items-center justify-center text-sm text-muted-foreground">
				Loading...
			</div>
		)
	}

	if (state.status === 'error') {
		return (
			<div className="flex h-screen flex-col items-center justify-center gap-3 p-4 text-center">
				<p className="text-sm text-destructive">
					Cannot reach the Nib backend: {state.message}
				</p>
				<Button variant="outline" onClick={() => void refresh()}>
					Retry
				</Button>
			</div>
		)
	}

	if (state.required && !state.authenticated) {
		return <LoginPage onSuccess={refresh} />
	}

	return <AuthContext.Provider value={contextValue}>{children}</AuthContext.Provider>
}
