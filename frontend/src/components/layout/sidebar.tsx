import type { LucideIcon } from 'lucide-react'
import { NavLink } from 'react-router'
import {
	BookOpen,
	Braces,
	Building2,
	ChevronsLeft,
	ChevronsRight,
	FileCode2,
	ShieldCheck,
	AlertTriangle,
	Wrench,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Separator } from '@/components/ui/separator'
import {
	Tooltip,
	TooltipTrigger,
	TooltipContent,
} from '@/components/ui/tooltip'
import { useSidebar } from './sidebar-context'
import { SidebarHelpMenu } from './sidebar-help-menu'
import nibLogo from '@/assets/nib-logo-transparent.png'

type NavItem = {
	to: string
	label: string
	icon: LucideIcon
	end?: boolean
	badge?: string
}

const WORKPLACE_NAV: NavItem[] = [
	{ to: '/workplace', label: 'Workplace', icon: Building2, end: true },
	{
		to: '/incidents',
		label: 'Incidents',
		icon: AlertTriangle,
		end: true,
		badge: 'In dev',
	},
]

const NAV_ITEMS: NavItem[] = [
	{ to: '/knowledge', label: 'Knowledgebase', icon: BookOpen },
	{ to: '/rules', label: 'Rules', icon: ShieldCheck },
	{ to: '/tools', label: 'Tools', icon: Wrench },
	{ to: '/skills', label: 'Skills', icon: FileCode2 },
	{ to: '/variables', label: 'Variables', icon: Braces },
]

function navLinkClassName(
	collapsed: boolean,
	isActive: boolean,
	nested: boolean,
) {
	return cn(
		'flex items-center rounded-md text-sm font-medium transition-colors',
		collapsed ? 'justify-center p-2' : 'gap-3 py-2',
		!collapsed && (nested ? 'pl-9 pr-3' : 'px-3'),
		isActive
			? 'bg-sidebar-accent text-sidebar-accent-foreground'
			: 'text-sidebar-foreground/70 hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground',
	)
}

function SidebarNavItem({
	to,
	label,
	icon: Icon,
	end,
	badge,
	collapsed,
	nested = false,
}: NavItem & { collapsed: boolean; nested?: boolean }) {
	const link = (
		<NavLink
			to={to}
			end={end}
			className={({ isActive }) =>
				navLinkClassName(collapsed, isActive, nested)
			}
		>
			<div className="relative shrink-0">
				<Icon className="h-4 w-4" />
			</div>
			{!collapsed && <span>{label}</span>}
			{!collapsed && badge && (
				<span className="ml-auto rounded-full bg-amber-500/15 px-2 py-0.5 text-[10px] font-semibold uppercase leading-none tracking-wide text-amber-600 dark:text-amber-400">
					{badge}
				</span>
			)}
		</NavLink>
	)

	if (collapsed) {
		return (
			<Tooltip>
				<TooltipTrigger asChild>
					<div>{link}</div>
				</TooltipTrigger>
				<TooltipContent side="right" sideOffset={8}>
					{label}
					{badge && (
						<span className="ml-2 text-amber-500">({badge})</span>
					)}
				</TooltipContent>
			</Tooltip>
		)
	}

	return <div>{link}</div>
}

export function Sidebar() {
	const { collapsed, toggle } = useSidebar()

	return (
		<aside
			className={cn(
				'glass-panel relative z-20 flex h-full shrink-0 flex-col border-r text-sidebar-foreground transition-[width] duration-200 ease-in-out',
				collapsed ? 'w-16' : 'w-60',
			)}
		>
			<div
				className={cn(
					'flex h-14 shrink-0 items-center',
					collapsed ? 'justify-center' : 'px-4',
				)}
			>
				{collapsed ? (
					<div className="h-8 w-8 overflow-hidden">
						<img
							src={nibLogo}
							alt="Nib"
							className="h-8 w-auto max-w-none"
						/>
					</div>
				) : (
					<img
						src={nibLogo}
						alt="Nib"
						className="h-8 w-auto"
					/>
				)}
			</div>

			<Separator className="bg-[oklch(1_0_0/0.45)] dark:bg-[oklch(1_0_0/0.12)]" />

			<nav className="flex-1 space-y-1 overflow-y-auto p-2">
				{WORKPLACE_NAV.map((item) => (
					<SidebarNavItem
						key={item.to}
						{...item}
						collapsed={collapsed}
					/>
				))}
				{NAV_ITEMS.map((item) => (
					<SidebarNavItem key={item.to} {...item} collapsed={collapsed} />
				))}
			</nav>

			<Separator className="bg-[oklch(1_0_0/0.45)] dark:bg-[oklch(1_0_0/0.12)]" />

			<div className="shrink-0 space-y-1 p-2">
				<SidebarHelpMenu collapsed={collapsed} />
				{collapsed ? (
					<Tooltip>
						<TooltipTrigger asChild>
							<button
								type="button"
								onClick={toggle}
								className="flex w-full cursor-pointer items-center justify-center rounded-md p-2 text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
							>
								<ChevronsRight className="h-4 w-4" />
							</button>
						</TooltipTrigger>
						<TooltipContent side="right" sideOffset={8}>
							Expand sidebar
						</TooltipContent>
					</Tooltip>
				) : (
					<button
						type="button"
						onClick={toggle}
						className="flex w-full cursor-pointer items-center gap-3 rounded-md px-3 py-2 text-sm font-medium text-sidebar-foreground/70 transition-colors hover:bg-sidebar-accent/50 hover:text-sidebar-accent-foreground"
					>
						<ChevronsLeft className="h-4 w-4 shrink-0" />
						<span>Collapse</span>
					</button>
				)}
			</div>
		</aside>
	)
}
