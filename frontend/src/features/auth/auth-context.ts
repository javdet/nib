import { createContext, useContext } from 'react'

export interface AuthContextValue {
	/** Whether the backend demands a login at all; hides "Sign out" when not. */
	required: boolean
	signOut: () => Promise<void>
}

export const AuthContext = createContext<AuthContextValue>({
	required: false,
	signOut: async () => {},
})

export function useAuth(): AuthContextValue {
	return useContext(AuthContext)
}
