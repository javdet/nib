import {
	createContext,
	useContext,
	useState,
	useCallback,
	useEffect,
	type ReactNode,
} from 'react'
import {
	type Cloud,
	listClouds,
} from './api/clouds'
import { useProject } from './project-context'
import { useSelectableResource } from './use-selectable-resource'

interface CloudContextValue {
	clouds: Cloud[]
	selectedCloud: Cloud | null
	loading: boolean
	selectCloud: (cloud: Cloud) => void
	refreshClouds: () => Promise<void>
}

const CloudContext = createContext<CloudContextValue | null>(null)

export function CloudProvider({ children }: { children: ReactNode }) {
	const { selectedProject } = useProject()
	const [clouds, setClouds] = useState<Cloud[]>([])
	const [loading, setLoading] = useState(false)

	const refreshClouds = useCallback(async () => {
		if (!selectedProject) {
			setClouds([])
			return
		}
		setLoading(true)
		try {
			const data = await listClouds(selectedProject.id)
			setClouds(data)
		} catch {
			setClouds([])
		} finally {
			setLoading(false)
		}
	}, [selectedProject])

	useEffect(() => {
		void refreshClouds()
	}, [refreshClouds])

	const { selected: selectedCloud, select } = useSelectableResource(
		'selected-cloud-id',
		clouds,
	)

	const selectCloud = useCallback(
		(cloud: Cloud) => {
			select(cloud)
		},
		[select],
	)

	return (
		<CloudContext
			value={{
				clouds,
				selectedCloud,
				loading,
				selectCloud,
				refreshClouds,
			}}
		>
			{children}
		</CloudContext>
	)
}

export function useCloud() {
	const ctx = useContext(CloudContext)
	if (!ctx) {
		throw new Error('useCloud must be used within CloudProvider')
	}
	return ctx
}
