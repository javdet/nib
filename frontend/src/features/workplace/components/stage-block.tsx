import { useState, type ReactNode } from 'react'
import {
	ChevronDown,
	ChevronRight,
	Circle,
	CircleAlert,
	CircleCheck,
	FastForward,
	Hourglass,
	Loader2,
	MessageCircleQuestion,
	MessagesSquare,
	RefreshCw,
	Square,
} from 'lucide-react'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import type { StageStatus } from '../lib/stage-status'
import { HeaderButton } from './plan-row-parts'

function StageStatusIcon({ status }: { status: StageStatus }) {
	switch (status.kind) {
		case 'running':
			return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
		case 'planning':
			return <Loader2 className="h-4 w-4 animate-spin text-muted-foreground" />
		case 'queued':
			return <Hourglass className="h-4 w-4 text-muted-foreground" />
		case 'waiting':
			return (
				<MessageCircleQuestion className="h-4 w-4 text-amber-600 dark:text-amber-400" />
			)
		case 'complete':
			return <CircleCheck className="h-4 w-4 text-success" />
		case 'planning_failed':
		case 'attention':
			return <CircleAlert className="h-4 w-4 text-destructive" />
		case 'empty':
			return <Circle className="h-4 w-4 text-muted-foreground/60" />
		case 'progress':
			return (
				<span className="font-mono text-xs tabular-nums text-muted-foreground">
					{status.done}/{status.total}
				</span>
			)
	}
}

interface StageBlockProps {
	heading: ReactNode
	/** Sits beside the heading: the stage's impact rollup. */
	badge?: ReactNode
	status: StageStatus
	complete: boolean
	defaultExpanded?: boolean
	/** Opens the planner's chat for this stage; the button is off without one. */
	onOpenChat?: () => void
	/** Omitted while replanning is not on offer at all. */
	onReplan?: () => void
	runActive: boolean
	runDisabledReason: string | null
	/** A start or stop request is in flight. */
	runBusy?: boolean
	onRun: () => void
	onStop: () => void
	/** A line under the header, for why the last run stopped. */
	note?: ReactNode
	children?: ReactNode
}

// A stage is one block: a header that says where it stands and acts on the
// whole of it, and the stage's rows underneath, which keep their own controls.
// Collapsing leaves the header, so a long plan reads as its list of stages.
export function StageBlock({
	heading,
	badge,
	status,
	complete,
	defaultExpanded = true,
	onOpenChat,
	onReplan,
	runActive,
	runDisabledReason,
	runBusy = false,
	onRun,
	onStop,
	note,
	children,
}: StageBlockProps) {
	const [expanded, setExpanded] = useState(defaultExpanded)
	const hasBody = Boolean(children) || Boolean(note)
	const toggle = () => setExpanded((prev) => !prev)

	return (
		<section
			className={cn(
				'overflow-hidden rounded-lg border bg-card',
				complete && 'border-success-border',
			)}
		>
			<div
				className={cn(
					'flex items-center gap-1 py-1 pl-2 pr-1',
					expanded && hasBody && 'border-b',
					complete &&
						'bg-success-surface [--border:var(--success-border)]',
				)}
			>
				<div
					role="button"
					tabIndex={0}
					aria-expanded={expanded}
					className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-sm py-1 outline-none focus-visible:ring-2 focus-visible:ring-ring"
					onClick={toggle}
					onKeyDown={(e) => {
						if (e.key === 'Enter' || e.key === ' ') {
							e.preventDefault()
							toggle()
						}
					}}
				>
					{expanded ? (
						<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
					) : (
						<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
					)}
					<h3
						className={cn(
							'min-w-0 truncate text-base font-semibold',
							complete && 'text-success',
						)}
					>
						{heading}
					</h3>
					{badge}
				</div>

				<Tooltip>
					<TooltipTrigger asChild>
						<span
							tabIndex={0}
							aria-label={status.tooltip}
							className="flex h-9 min-w-9 shrink-0 items-center justify-center rounded-sm px-1 outline-none focus-visible:ring-2 focus-visible:ring-ring"
						>
							<StageStatusIcon status={status} />
						</span>
					</TooltipTrigger>
					<TooltipContent className="max-w-xs">{status.tooltip}</TooltipContent>
				</Tooltip>

				<HeaderButton
					label="Open chat"
					tooltip={onOpenChat ? "Open this stage's planning chat" : 'No planning chat for this stage yet'}
					onClick={() => onOpenChat?.()}
					disabled={!onOpenChat}
				>
					<MessagesSquare className="h-4 w-4" />
				</HeaderButton>

				{onReplan && (
					<HeaderButton label="Replan" tooltip="Replan this stage" onClick={onReplan}>
						<RefreshCw className="h-4 w-4" />
					</HeaderButton>
				)}

				{runActive ? (
					<HeaderButton
						label="Stop run"
						tooltip="Stop running this stage, and the item it is on"
						onClick={onStop}
						disabled={runBusy}
						active
					>
						<Square className="h-4 w-4" />
					</HeaderButton>
				) : (
					<HeaderButton
						label="Execute all"
						tooltip={
							runDisabledReason ??
							'Execute every unticked item in order, each after the one before it finishes'
						}
						onClick={onRun}
						disabled={runBusy || runDisabledReason !== null}
					>
						<span className="flex size-6 items-center justify-center rounded-full border border-current/40">
							<FastForward className="ml-px size-3 fill-current" />
						</span>
					</HeaderButton>
				)}
			</div>

			{expanded && hasBody && (
				<div className="space-y-3 p-3">
					{note}
					{children}
				</div>
			)}
		</section>
	)
}
