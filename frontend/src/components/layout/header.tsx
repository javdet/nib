import { useState } from 'react'
import {
	FolderKanban,
	Globe,
	Cloud,
	MapPin,
	SlidersHorizontal,
	ChevronsUpDown,
	Check,
} from 'lucide-react'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { useProject } from '@/features/projects/project-context'
import { useEnvironment } from '@/features/projects/environment-context'
import { useCloud } from '@/features/projects/cloud-context'
import { useLocation } from '@/features/projects/location-context'
import { useMode } from '@/features/modes/mode-context'

function formatModeLabel(mode: string): string {
	return mode.charAt(0).toUpperCase() + mode.slice(1)
}

function formatSelectionLabel(name: string | undefined): string {
	if (!name || name === 'any') return 'Any'
	return name
}

export function Header() {
	const [openMenu, setOpenMenu] = useState<string | null>(null)
	const { selectedMode } = useMode()
	const { projects, selectedProject, selectProject, loading } = useProject()
	const {
		environments,
		selectedEnvironment,
		selectEnvironment,
		loading: envLoading,
	} = useEnvironment()
	const {
		clouds,
		selectedCloud,
		selectCloud,
		loading: cloudLoading,
	} = useCloud()
	const {
		locations,
		selectedLocation,
		selectLocation,
		loading: locationLoading,
	} = useLocation()

	function handleSelectProject(project: typeof projects[number]) {
		selectProject(project)
	}

	const projectLabel = formatSelectionLabel(selectedProject?.name)
	const envLabel = formatSelectionLabel(selectedEnvironment?.name)
	const cloudLabel = formatSelectionLabel(selectedCloud?.name)
	const locationLabel = formatSelectionLabel(selectedLocation?.name)
	const modeLabel = formatModeLabel(selectedMode)

	return (
		<header className="glass-panel pointer-events-auto absolute inset-x-0 top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b px-6">
			<DropdownMenu
				open={openMenu === 'project'}
				onOpenChange={(open) => setOpenMenu(open ? 'project' : null)}
			>
				<DropdownMenuTrigger asChild>
					<button
						className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground"
						disabled={loading}
					>
						<FolderKanban className="h-4 w-4 shrink-0" />
						<span className="max-w-[200px] truncate">{projectLabel}</span>
						<ChevronsUpDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start" className="w-56">
					{projects.map((project) => (
						<DropdownMenuItem
							key={project.id}
							onClick={() => handleSelectProject(project)}
						>
							<span className="flex-1 truncate">{project.name}</span>
							{selectedProject?.id === project.id && (
								<Check className="ml-2 h-4 w-4 shrink-0" />
							)}
						</DropdownMenuItem>
					))}
				</DropdownMenuContent>
			</DropdownMenu>

			<DropdownMenu
				open={openMenu === 'environment'}
				onOpenChange={(open) => setOpenMenu(open ? 'environment' : null)}
			>
				<DropdownMenuTrigger asChild>
					<button
						className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
						disabled={envLoading}
					>
						<Globe className="h-4 w-4 shrink-0" />
						<span className="max-w-[200px] truncate">{envLabel}</span>
						<ChevronsUpDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start" className="w-56">
					{environments.map((env) => (
						<DropdownMenuItem
							key={env.id}
							onClick={() => selectEnvironment(env)}
						>
							<span className="flex-1 truncate">{env.name}</span>
							{selectedEnvironment?.id === env.id && (
								<Check className="ml-2 h-4 w-4 shrink-0" />
							)}
						</DropdownMenuItem>
					))}
				</DropdownMenuContent>
			</DropdownMenu>

			<DropdownMenu
				open={openMenu === 'cloud'}
				onOpenChange={(open) => setOpenMenu(open ? 'cloud' : null)}
			>
				<DropdownMenuTrigger asChild>
					<button
						className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
						disabled={cloudLoading}
					>
						<Cloud className="h-4 w-4 shrink-0" />
						<span className="max-w-[200px] truncate">{cloudLabel}</span>
						<ChevronsUpDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start" className="w-56">
					{clouds.map((cloud) => (
						<DropdownMenuItem
							key={cloud.id}
							onClick={() => selectCloud(cloud)}
						>
							<span className="flex-1 truncate">{cloud.name}</span>
							{selectedCloud?.id === cloud.id && (
								<Check className="ml-2 h-4 w-4 shrink-0" />
							)}
						</DropdownMenuItem>
					))}
				</DropdownMenuContent>
			</DropdownMenu>

			<DropdownMenu
				open={openMenu === 'location'}
				onOpenChange={(open) => setOpenMenu(open ? 'location' : null)}
			>
				<DropdownMenuTrigger asChild>
					<button
						className="flex cursor-pointer items-center gap-2 rounded-md px-2 py-1.5 text-sm font-medium transition-colors hover:bg-accent hover:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
						disabled={locationLoading}
					>
						<MapPin className="h-4 w-4 shrink-0" />
						<span className="max-w-[200px] truncate">{locationLabel}</span>
						<ChevronsUpDown className="h-3.5 w-3.5 shrink-0 opacity-50" />
					</button>
				</DropdownMenuTrigger>
				<DropdownMenuContent align="start" className="w-56">
					{locations.map((location) => (
						<DropdownMenuItem
							key={location.id}
							onClick={() => selectLocation(location)}
						>
							<span className="flex-1 truncate">{location.name}</span>
							{selectedLocation?.id === location.id && (
								<Check className="ml-2 h-4 w-4 shrink-0" />
							)}
						</DropdownMenuItem>
					))}
				</DropdownMenuContent>
			</DropdownMenu>

			<div
				className="ml-auto flex items-center gap-2 rounded-full border border-primary/25 bg-primary/8 px-3.5 py-1.5 text-sm font-semibold tracking-wide text-foreground shadow-sm ring-1 ring-primary/10"
				aria-label={`Mode: ${modeLabel}`}
			>
				<SlidersHorizontal className="h-4 w-4 shrink-0 text-primary" />
				<span className="max-w-[200px] truncate">{modeLabel}</span>
			</div>
		</header>
	)
}
