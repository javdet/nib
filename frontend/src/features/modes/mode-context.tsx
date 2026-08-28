import {
	createContext,
	useContext,
	useState,
	useCallback,
	type ReactNode,
} from 'react'

export const DEFAULT_MODE = 'discuss'

interface ModeContextValue {
	selectedMode: string
	selectMode: (mode: string) => void
}

const ModeContext = createContext<ModeContextValue | null>(null)

export function ModeProvider({ children }: { children: ReactNode }) {
	const [selectedMode, setSelectedMode] = useState<string>(DEFAULT_MODE)

	const selectMode = useCallback((mode: string) => {
		setSelectedMode(mode)
	}, [])

	return (
		<ModeContext
			value={{
				selectedMode,
				selectMode,
			}}
		>
			{children}
		</ModeContext>
	)
}

export function useMode() {
	const ctx = useContext(ModeContext)
	if (!ctx) throw new Error('useMode must be used within ModeProvider')
	return ctx
}
