import { useEffect } from 'react'
import { useProject } from './project-context'
import { useEnvironment } from './environment-context'
import { useCloud } from './cloud-context'
import { useLocation } from './location-context'
import { setSelection } from './api/selection'

export function SelectionSync() {
	const { selectedProject } = useProject()
	const { selectedEnvironment } = useEnvironment()
	const { selectedCloud } = useCloud()
	const { selectedLocation } = useLocation()

	useEffect(() => {
		if (
			!selectedProject ||
			!selectedEnvironment ||
			!selectedCloud ||
			!selectedLocation
		) {
			return
		}

		void setSelection({
			project: selectedProject.name,
			environment: selectedEnvironment.name,
			cloud: selectedCloud.name,
			location: selectedLocation.name,
		}).catch((err) => {
			console.error('[SelectionSync] failed to sync selection:', err)
		})
	}, [
		selectedProject,
		selectedEnvironment,
		selectedCloud,
		selectedLocation,
	])

	return null
}
