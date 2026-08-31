import { useState } from 'react'
import { BookOpen, CircleHelp, ExternalLink, Info } from 'lucide-react'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import { AboutDialog } from '@/features/system/components/about-dialog'
import { cn } from '@/lib/utils'

const DOCUMENTATION_URL = 'https://github.com/javdet/nib/tree/main/docs'
const CHANGELOG_URL = 'https://github.com/javdet/nib/releases'

type SidebarHelpMenuProps = {
	collapsed: boolean
}

function openInNewTab(url: string) {
	window.open(url, '_blank', 'noopener,noreferrer')
}

export function SidebarHelpMenu({ collapsed }: SidebarHelpMenuProps) {
	const [aboutOpen, setAboutOpen] = useState(false)

	const triggerClassName = cn(
		'flex w-full cursor-pointer items-center rounded-md text-sm font-medium',
		'text-sidebar-foreground/70 transition-colors duration-[var(--dur-fast)]',
		'focus-ring hover:bg-foreground/[0.06] hover:text-sidebar-accent-foreground',
		collapsed ? 'justify-center p-2' : 'gap-3 px-3 py-2',
	)

	const trigger = (
		<DropdownMenuTrigger asChild>
			<button type="button" className={triggerClassName}>
				<CircleHelp className="h-4 w-4 shrink-0" />
				{!collapsed && <span>Help</span>}
			</button>
		</DropdownMenuTrigger>
	)

	return (
		<>
			<DropdownMenu>
				{collapsed ? (
					<Tooltip>
						<TooltipTrigger asChild>{trigger}</TooltipTrigger>
						<TooltipContent side="right" sideOffset={8}>
							Help
						</TooltipContent>
					</Tooltip>
				) : (
					trigger
				)}
				<DropdownMenuContent
					side="right"
					align="end"
					sideOffset={8}
					className="w-52"
				>
					<DropdownMenuItem
						onSelect={() => openInNewTab(DOCUMENTATION_URL)}
					>
						<BookOpen />
						Documentation
					</DropdownMenuItem>
					<DropdownMenuItem
						onSelect={() => openInNewTab(CHANGELOG_URL)}
					>
						<ExternalLink />
						Changelog
					</DropdownMenuItem>
					<DropdownMenuItem onSelect={() => setAboutOpen(true)}>
						<Info />
						About Nib
					</DropdownMenuItem>
				</DropdownMenuContent>
			</DropdownMenu>

			<AboutDialog open={aboutOpen} onOpenChange={setAboutOpen} />
		</>
	)
}
