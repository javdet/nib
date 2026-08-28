import {
	createContext,
	useContext,
	useState,
	useCallback,
	useEffect,
	type ReactNode,
} from 'react'
import {
	type Project,
	listProjects,
} from './api/projects'
import { useSelectableResource } from './use-selectable-resource'

interface ProjectContextValue {
	projects: Project[]
	selectedProject: Project | null
	loading: boolean
	selectProject: (project: Project) => void
	refreshProjects: () => Promise<void>
}

const ProjectContext = createContext<ProjectContextValue | null>(null)

export function ProjectProvider({ children }: { children: ReactNode }) {
	const [projects, setProjects] = useState<Project[]>([])
	const [loading, setLoading] = useState(true)

	const refreshProjects = useCallback(async () => {
		setLoading(true)
		try {
			const data = await listProjects()
			setProjects(data)
		} catch (err) {
			console.error('[ProjectProvider] failed to load projects:', err)
		} finally {
			setLoading(false)
		}
	}, [])

	useEffect(() => {
		void refreshProjects()
	}, [refreshProjects])

	const { selected: selectedProject, select } = useSelectableResource(
		'selected-project-id',
		projects,
	)

	const selectProject = useCallback(
		(project: Project) => {
			select(project)
		},
		[select],
	)

	return (
		<ProjectContext
			value={{
				projects,
				selectedProject,
				loading,
				selectProject,
				refreshProjects,
			}}
		>
			{children}
		</ProjectContext>
	)
}

export function useProject() {
	const ctx = useContext(ProjectContext)
	if (!ctx) throw new Error('useProject must be used within ProjectProvider')
	return ctx
}
