import {
	createContext,
	useContext,
	useState,
	useCallback,
	useEffect,
	type ReactNode,
} from 'react'
import {
	type Environment,
	listEnvironments,
} from './api/environments'
import { useProject } from './project-context'
import { useSelectableResource } from './use-selectable-resource'

interface EnvironmentContextValue {
	environments: Environment[]
	selectedEnvironment: Environment | null
	loading: boolean
	selectEnvironment: (env: Environment) => void
	refreshEnvironments: () => Promise<void>
}

const EnvironmentContext = createContext<EnvironmentContextValue | null>(null)

export function EnvironmentProvider({ children }: { children: ReactNode }) {
	const { selectedProject } = useProject()
	const [environments, setEnvironments] = useState<Environment[]>([])
	const [loading, setLoading] = useState(false)

	const refreshEnvironments = useCallback(async () => {
		if (!selectedProject) {
			setEnvironments([])
			return
		}
		setLoading(true)
		try {
			const data = await listEnvironments(selectedProject.id)
			setEnvironments(data)
		} catch {
			setEnvironments([])
		} finally {
			setLoading(false)
		}
	}, [selectedProject])

	useEffect(() => {
		void refreshEnvironments()
	}, [refreshEnvironments])

	const { selected: selectedEnvironment, select } = useSelectableResource(
		'selected-environment-id',
		environments,
	)

	const selectEnvironment = useCallback(
		(env: Environment) => {
			select(env)
		},
		[select],
	)

	return (
		<EnvironmentContext
			value={{
				environments,
				selectedEnvironment,
				loading,
				selectEnvironment,
				refreshEnvironments,
			}}
		>
			{children}
		</EnvironmentContext>
	)
}

export function useEnvironment() {
	const ctx = useContext(EnvironmentContext)
	if (!ctx) {
		throw new Error(
			'useEnvironment must be used within EnvironmentProvider',
		)
	}
	return ctx
}
