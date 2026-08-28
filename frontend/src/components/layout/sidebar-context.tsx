import { createContext, useContext, useState, useCallback, type ReactNode } from 'react'

interface SidebarContextValue {
	collapsed: boolean
	toggle: () => void
}

const SidebarContext = createContext<SidebarContextValue | null>(null)

export function SidebarProvider({ children }: { children: ReactNode }) {
	const [collapsed, setCollapsed] = useState(() => {
		return localStorage.getItem('sidebar-collapsed') === 'true'
	})

	const toggle = useCallback(() => {
		setCollapsed((prev) => {
			const next = !prev
			localStorage.setItem('sidebar-collapsed', String(next))
			return next
		})
	}, [])

	return (
		<SidebarContext value={{ collapsed, toggle }}>
			{children}
		</SidebarContext>
	)
}

export function useSidebar() {
	const ctx = useContext(SidebarContext)
	if (!ctx) throw new Error('useSidebar must be used within SidebarProvider')
	return ctx
}
