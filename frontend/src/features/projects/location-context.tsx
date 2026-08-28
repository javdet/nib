import {
	createContext,
	useContext,
	useState,
	useCallback,
	useEffect,
	type ReactNode,
} from 'react'
import {
	type Location,
	listLocations,
} from './api/locations'
import { useProject } from './project-context'
import { useCloud } from './cloud-context'
import { useSelectableResource } from './use-selectable-resource'

interface LocationContextValue {
	locations: Location[]
	selectedLocation: Location | null
	loading: boolean
	selectLocation: (location: Location) => void
	refreshLocations: () => Promise<void>
}

const LocationContext = createContext<LocationContextValue | null>(null)

export function LocationProvider({ children }: { children: ReactNode }) {
	const { selectedProject } = useProject()
	const { selectedCloud } = useCloud()
	const [locations, setLocations] = useState<Location[]>([])
	const [loading, setLoading] = useState(false)

	const refreshLocations = useCallback(async () => {
		if (!selectedProject || !selectedCloud) {
			setLocations([])
			return
		}
		setLoading(true)
		try {
			const data = await listLocations(selectedProject.id, selectedCloud.id)
			setLocations(data)
		} catch {
			setLocations([])
		} finally {
			setLoading(false)
		}
	}, [selectedProject, selectedCloud])

	useEffect(() => {
		void refreshLocations()
	}, [refreshLocations])

	const { selected: selectedLocation, select } = useSelectableResource(
		'selected-location-id',
		locations,
	)

	const selectLocation = useCallback(
		(location: Location) => {
			select(location)
		},
		[select],
	)

	return (
		<LocationContext
			value={{
				locations,
				selectedLocation,
				loading,
				selectLocation,
				refreshLocations,
			}}
		>
			{children}
		</LocationContext>
	)
}

export function useLocation() {
	const ctx = useContext(LocationContext)
	if (!ctx) {
		throw new Error('useLocation must be used within LocationProvider')
	}
	return ctx
}
