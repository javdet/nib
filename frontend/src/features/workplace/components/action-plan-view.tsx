import {
	useCallback,
	useEffect,
	useLayoutEffect,
	useRef,
	useState,
	type DragEvent,
	type ReactNode,
} from 'react'
import {
	AlertTriangle,
	ChevronDown,
	ChevronUp,
	FolderGit2,
	GripVertical,
	Loader2,
	MessageSquare,
	OctagonAlert,
	Pencil,
	Play,
	ScrollText,
	Square,
	type LucideIcon,
} from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { CopyButton } from '@/components/copy-button'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import { MarkdownMessage } from '@/components/markdown-message'
import { cn } from '@/lib/utils'
import type {
	ActionExecRun,
	ActionExecRuns,
	ActionPlan,
	ActionStep,
	FanoutRun,
	StageRun,
	StageRunScope,
} from '@/features/dialogs/api/dialogs'
import type { ActionPlanScope } from '@/features/dialogs/api/dialogs'
import type { ExecutorType } from '@/features/executor/api/executor'
import { actionTypeIcon } from '../lib/action-type-icon'
import { DONE_SURFACE, HeaderButton } from './plan-row-parts'
import { canViewContainerLogs } from '../lib/container-logs'
import {
	impactedIndexes,
	stepImpact,
	strongestImpact,
	type ImpactKind,
} from '../lib/step-impact'
import {
	actionPlanItemNumber,
	actionPlanNumberForKey,
	actionPlanStageNumber,
} from '../lib/action-plan-number'
import {
	deriveStageStatus,
	findFanoutRollback,
	isItemsComplete,
	isStageRunFor,
	mergePlanStages,
	rollbackItemKeys,
	runAllDisabledReason,
	stageItemKeys,
} from '../lib/stage-status'
import { StageBlock } from './stage-block'

// The status of a sub-agent run, shown beside the row it belongs to. The record
// is separate from the checkbox on purpose: "the sub-agent finished" is a weaker
// claim than "this action is done", which stays the operator's to make.
function ExecStatusDot({ run }: { run: ActionExecRun }) {
	if (run.status === 'running') {
		return <Loader2 className="h-4 w-4 animate-spin text-blue-500" />
	}

	const color =
		run.status === 'done'
			? 'bg-success'
			: run.status === 'blocked'
				? 'bg-amber-500'
				: run.status === 'cancelled'
					? 'bg-muted-foreground'
					: 'bg-destructive'
	return <span className={cn('h-2.5 w-2.5 rounded-full', color)} />
}

function execRunTooltip(run: ActionExecRun): string {
	const attempt = run.attempt > 1 ? ` (attempt ${run.attempt})` : ''
	switch (run.status) {
		case 'running':
			return `Sub-agent running${attempt}`
		case 'done':
			return `Sub-agent finished${attempt} — tick the box yourself once you are satisfied`
		case 'blocked':
			return run.error
				? `Needs a decision${attempt}: ${run.error}`
				: `Needs a decision${attempt}`
		case 'cancelled':
			return `Cancelled${attempt}`
		default:
			return run.error ? `Failed${attempt}: ${run.error}` : `Failed${attempt}`
	}
}

const EXECUTOR_DISABLED_REASON =
	'Executor is disabled — set its type in executor settings to run code actions'

// Rows open collapsed so a ten-step plan stays scannable. The cut is a height
// rather than a sentence count: an action is markdown, and lists, tables and
// code blocks have no sentences to count but do have lines to clip. Roughly six
// lines of prose at the body's leading.
const COLLAPSED_BODY_PX = 132

interface ActionPlanViewProps {
	// plan is null while the first planning run has not written a stage yet; the
	// stages it is working on are drawn from fanoutRun instead.
	plan: ActionPlan | null
	checked: string[]
	comments: Record<string, string>
	onToggle: (key: string, nextChecked: boolean) => void
	onComment: (key: string) => void
	onEdit: (key: string) => void
	onExecute: (key: string) => void
	// onStop force-stops the execution running right now. It is the way out of a
	// run that is wedged, so it is offered on every running row rather than only
	// on the one the operator started.
	onStop: () => void
	// onViewLogs opens the container logs of one running code action. Its row
	// key is passed back because the modal reads them per action.
	onViewLogs?: (key: string) => void
	execRuns: ActionExecRuns
	// executorType gates the logs button: only the local docker executor can be
	// read from.
	executorType?: ExecutorType | null
	onReorder: (scope: ActionPlanScope, stage: number, from: number, to: number) => void
	reordering?: boolean
	// executorDisabled mirrors the "disabled" executor type: code actions have
	// nowhere to run, so their Execute button is turned off.
	executorDisabled?: boolean
	// The planning run supplies each stage header's planning status and the
	// planner chat it opens.
	fanoutRun?: FanoutRun | null
	fanoutRunning?: boolean
	stageRun?: StageRun | null
	// stageRunBusy is a start or stop request in flight.
	stageRunBusy?: boolean
	canReplan?: boolean
	onOpenStageChat?: (dialogId: string) => void
	onReplanStage?: (title: string) => void
	onReplanRollback?: () => void
	onRunStage?: (scope: StageRunScope, stage: number) => void
	onStopStageRun?: () => void
}

function isCodeStep(step: ActionStep): boolean {
	return step.type?.trim().toLowerCase() === 'code'
}

interface DragState {
	scope: ActionPlanScope
	stage: number
	index: number
}

interface DragHandleProps {
	disabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
}

function DragHandle({
	disabled,
	onGripPointerDown,
	onGripPointerUp,
}: DragHandleProps) {
	return (
		<div
			className={cn(
				'flex h-9 shrink-0 select-none items-center justify-center border-t',
				disabled
					? 'cursor-not-allowed text-muted-foreground/40'
					: 'cursor-grab text-muted-foreground hover:bg-muted hover:text-foreground active:cursor-grabbing',
			)}
			onPointerDown={(e) => {
				if (disabled) return
				e.stopPropagation()
				onGripPointerDown()
			}}
			onPointerUp={onGripPointerUp}
			aria-hidden={disabled}
		>
			<GripVertical className="h-4 w-4" />
		</div>
	)
}

interface RowGutterProps {
	id: string
	number: string
	checked: boolean
	onToggle: (key: string, nextChecked: boolean) => void
	dragDisabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
}

// The gutter stacks the three things that address a row rather than act on it:
// which item it is, whether it is done, and the grip that moves it. Keeping them
// in one column leaves the whole width of the row to the action itself.
function RowGutter({
	id,
	number,
	checked,
	onToggle,
	dragDisabled,
	onGripPointerDown,
	onGripPointerUp,
}: RowGutterProps) {
	return (
		<div className="flex w-10 shrink-0 flex-col self-stretch border-r">
			<div
				id={`${id}-number`}
				className={cn(
					'flex h-9 shrink-0 select-none items-center justify-center',
					'font-mono text-xs tabular-nums',
					checked ? 'text-success' : 'text-muted-foreground',
				)}
			>
				{number}
			</div>
			<div className="flex flex-1 items-start justify-center py-2">
				<Checkbox
					checked={checked}
					onCheckedChange={(value) => onToggle(id, value === true)}
					aria-labelledby={`${id}-number ${id}-label`}
					className={cn(
						'cursor-pointer',
						'data-[state=checked]:border-success data-[state=checked]:bg-success',
						'data-[state=checked]:text-success-foreground',
					)}
				/>
			</div>
			<DragHandle
				disabled={dragDisabled}
				onGripPointerDown={onGripPointerDown}
				onGripPointerUp={onGripPointerUp}
			/>
		</div>
	)
}

// CollapsibleBody clips its content to COLLAPSED_BODY_PX and offers a toggle,
// but only once the content genuinely overflows -- a two-line action gets no
// furniture it does not need.
function CollapsibleBody({ id, children }: { id: string; children: ReactNode }) {
	const contentRef = useRef<HTMLDivElement>(null)
	const [expanded, setExpanded] = useState(false)
	const [overflows, setOverflows] = useState(false)

	useLayoutEffect(() => {
		const el = contentRef.current
		if (!el) return

		// Measured on the inner, unclamped element: the outer wrapper's height is
		// the answer we imposed, so asking it would only echo the clamp back.
		const measure = () => {
			setOverflows(el.scrollHeight > COLLAPSED_BODY_PX + 4)
		}
		measure()

		// Markdown settles late -- tables reflow, long code wraps -- so remeasure
		// rather than trusting the first pass.
		const observer = new ResizeObserver(measure)
		observer.observe(el)
		return () => observer.disconnect()
	}, [])

	const collapsed = overflows && !expanded

	return (
		<>
			<div
				id={id}
				className={cn(
					'overflow-hidden',
					// A mask rather than a gradient overlay, so the fade works over the
					// muted surface and the emerald one without knowing which it is on.
					collapsed &&
						'[mask-image:linear-gradient(to_bottom,black_calc(100%_-_2rem),transparent)]',
				)}
				style={{ maxHeight: collapsed ? COLLAPSED_BODY_PX : undefined }}
			>
				<div ref={contentRef}>{children}</div>
			</div>
			{overflows ? (
				<div className="mt-1 flex justify-end">
					<button
						type="button"
						onClick={() => setExpanded((prev) => !prev)}
						aria-expanded={expanded}
						aria-controls={id}
						className={cn(
							'flex cursor-pointer items-center gap-0.5 rounded text-xs',
							'text-muted-foreground transition-colors focus-ring',
							'duration-[var(--dur-fast)] hover:text-foreground',
						)}
					>
						{expanded ? 'less' : 'more...'}
						{expanded ? (
							<ChevronUp className="size-3.5" aria-hidden />
						) : (
							<ChevronDown className="size-3.5" aria-hidden />
						)}
					</button>
				</div>
			) : null}
		</>
	)
}

// The repository rides in the bar beside the type tile, so it is sized to that
// bar rather than to the badge scale the impact labels use.
function RepositoryBadge({ step }: { step: ActionStep }) {
	const repository = step.repository?.trim()
	if (!isCodeStep(step) || !repository) return null

	return (
		<span
			className={cn(
				'inline-flex h-5 max-w-full items-center gap-1.5 rounded-md border',
				'border-border bg-background/40 px-2 text-xs text-foreground',
			)}
			title={`repository: ${repository}`}
		>
			<FolderGit2 className="size-3.5 shrink-0" aria-hidden />
			<span className="truncate">{repository}</span>
		</span>
	)
}

// The two impact labels. Colours come from the plan status pills so the whole
// view speaks one palette, and the wording is the operator's, not ours: the
// planner's sentence is the tooltip, because "downtime" alone does not say what
// goes down or for how long.
const IMPACT_STYLES: Record<
	ImpactKind,
	{ label: string; className: string; Icon: LucideIcon }
> = {
	downtime: {
		label: 'downtime',
		className:
			'border-red-500/30 bg-red-500/15 text-red-700 dark:text-red-300',
		Icon: OctagonAlert,
	},
	degraded: {
		label: 'degraded',
		className:
			'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300',
		Icon: AlertTriangle,
	},
}

function ImpactBadge({ step }: { step: ActionStep }) {
	const impact = stepImpact(step)
	if (!impact) return null

	const { label, className, Icon } = IMPACT_STYLES[impact.kind]

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge
					variant="outline"
					tabIndex={0}
					className={cn(
						'shrink-0 font-medium uppercase tracking-wide',
						className,
					)}
				>
					<Icon className="size-3.5 shrink-0" aria-hidden />
					{label}
				</Badge>
			</TooltipTrigger>
			{/* These tooltips hold a whole sentence, unlike every other one in
			    this file, so they need a width to wrap against. */}
			<TooltipContent className="max-w-xs">{impact.text}</TooltipContent>
		</Tooltip>
	)
}

// ActionRowHeader fills the row's header strip. The strip itself carries no
// gap, so the badges bring their own.
function ActionRowHeader({ step }: { step: ActionStep }) {
	return (
		<div className="flex min-w-0 items-center gap-2">
			<ImpactBadge step={step} />
		</div>
	)
}

// StageImpactBadge is the rollup on a stage heading: a collapsed or scrolled
// plan still says which stages take something down, and the tooltip names the
// rows so the operator can go straight to them.
function StageImpactBadge({
	steps,
	numberOf,
}: {
	steps: ActionStep[]
	numberOf: (index: number) => string
}) {
	const kind = strongestImpact(steps)
	if (!kind) return null

	const { label, className, Icon } = IMPACT_STYLES[kind]
	const numbers = impactedIndexes(steps, kind).map(numberOf)

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<Badge
					variant="outline"
					tabIndex={0}
					className={cn(
						'shrink-0 px-2 text-[10px] font-medium uppercase tracking-wide',
						className,
					)}
				>
					<Icon className="size-3 shrink-0" aria-hidden />
					{label}
				</Badge>
			</TooltipTrigger>
			<TooltipContent className="max-w-xs">
				{`${label === 'downtime' ? 'Downtime' : 'Degradation'} in ${numbers.join(', ')}`}
			</TooltipContent>
		</Tooltip>
	)
}

// The type icon sits in the body's top-left corner and is floated, so the
// narrative runs beside its lower half and then wraps back under it.
function ActionTypeTile({ type }: { type: string }) {
	const TypeIcon = actionTypeIcon(type)

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span
					tabIndex={0}
					aria-label={type}
					className={cn(
						'float-left mr-3 flex size-10 shrink-0 items-center justify-center',
						'rounded-md border bg-background/60 text-muted-foreground',
					)}
				>
					<TypeIcon className="size-5" aria-hidden />
				</span>
			</TooltipTrigger>
			<TooltipContent>{type}</TooltipContent>
		</Tooltip>
	)
}

// ActionBody renders the step narrative and, for shell and curl steps, the
// commands themselves as a copyable block.
function ActionBody({ step }: { step: ActionStep }) {
	const command = step.command?.trim()
	const type = step.type?.trim()
	const prURL = step.pr_url?.trim()
	const hasRepository = Boolean(isCodeStep(step) && step.repository?.trim())

	return (
		// The clearfix is what makes the body enclose the floated tile, so a
		// one-line narrative still measures tall enough to hold it.
		<div className="min-w-0 after:block after:clear-both after:content-['']">
			{type ? <ActionTypeTile type={type} /> : null}
			{/* A bar half the tile's height: it carries the repository when the
			    step has one, and either way it is what drops the narrative down
			    to the tile's lower half before the text wraps back under it. */}
			{type || hasRepository ? (
				<div className="flex h-5 min-w-0 items-center">
					<RepositoryBadge step={step} />
				</div>
			) : null}
			{/* Only prose wraps around the tile. A list, a rule or a quote is a
			    block, so it would slide underneath the tile and put its bullets
			    and borders behind it -- those start below it instead. Code
			    blocks and tables scroll, which already keeps them clear. */}
			<div className="[&_ul]:clear-left [&_ol]:clear-left [&_blockquote]:clear-left [&_hr]:clear-left">
				<MarkdownMessage content={step.action} />
			</div>
			{/* Clearing keeps a short narrative from leaving these beside the
			    tile, where they would read as its caption. */}
			{isCodeStep(step) ? (
				<div className="clear-left mt-2 text-xs">
					<span className="text-muted-foreground">Pull request: </span>
					{prURL ? (
						<a
							href={prURL}
							target="_blank"
							rel="noreferrer noopener"
							className="break-all underline underline-offset-2"
						>
							{prURL}
						</a>
					) : (
						<span className="text-muted-foreground">—</span>
					)}
				</div>
			) : null}
			{command ? (
				<div className="clear-left mt-2 flex items-start gap-1 rounded-md border bg-background/70 py-1 pl-2 pr-1">
					<code className="min-w-0 flex-1 overflow-x-auto whitespace-pre py-1 font-mono text-xs leading-relaxed">
						{command}
					</code>
					<CopyButton text={command} label="Copy command" />
				</div>
			) : null}
		</div>
	)
}

interface RowBodyProps {
	id: string
	checked: boolean
	children: ReactNode
}

function RowBody({ id, checked, children }: RowBodyProps) {
	return (
		<div
			id={`${id}-label`}
			className={cn(
				'min-w-0 select-text px-3 py-2',
				checked && 'text-foreground/75',
			)}
		>
			<CollapsibleBody id={`${id}-body`}>{children}</CollapsibleBody>
		</div>
	)
}

interface RunControlsProps {
	execRun?: ActionExecRun
	onStop: () => void
	onViewLogs?: () => void
	executorType?: ExecutorType | null
}

// The two controls every executable row shows about a run in flight: what it is
// doing and the way out of it. Stop is offered on any running row rather than
// only the one this operator started -- it is the only way out of a wedged
// execution, which holds the single slot for every plan.
function RunControls({
	execRun,
	onStop,
	onViewLogs,
	executorType = null,
}: RunControlsProps) {
	if (!execRun) return null

	return (
		<>
			<Tooltip>
				<TooltipTrigger asChild>
					<span className="flex h-9 w-9 shrink-0 items-center justify-center">
						<ExecStatusDot run={execRun} />
					</span>
				</TooltipTrigger>
				<TooltipContent>{execRunTooltip(execRun)}</TooltipContent>
			</Tooltip>
			{onViewLogs && canViewContainerLogs(execRun, executorType) && (
				<HeaderButton
					label="View container logs"
					tooltip="View container logs"
					onClick={onViewLogs}
				>
					<ScrollText className="h-4 w-4" />
				</HeaderButton>
			)}
			{execRun.status === 'running' && (
				<HeaderButton
					label="Stop execution"
					tooltip="Force-stop this execution and free the slot"
					onClick={onStop}
				>
					<Square className="h-4 w-4" />
				</HeaderButton>
			)}
		</>
	)
}

interface ExecuteButtonProps {
	onClick: () => void
	disabled?: boolean
	disabledReason?: string
}

function ExecuteButton({ onClick, disabled, disabledReason }: ExecuteButtonProps) {
	return (
		<HeaderButton
			label="Execute action"
			tooltip={disabled && disabledReason ? disabledReason : 'Execute action'}
			onClick={onClick}
			disabled={disabled}
		>
			<span className="flex size-6 items-center justify-center rounded-full border border-current/40">
				<Play className="ml-0.5 size-3 fill-current" />
			</span>
		</HeaderButton>
	)
}

interface CheckableRowProps {
	id: string
	number: string
	checked: boolean
	onToggle: (key: string, nextChecked: boolean) => void
	onExecute: () => void
	onStop: () => void
	execRun?: ActionExecRun
	dragDisabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
	children: ReactNode
}

// A check is executed like an action -- a sub-agent takes the reading and reports
// what it found -- so the row carries the same Execute button and run status. It
// keeps the dashed surface and skips Edit and Comment: a check is two fields the
// planner owns, and neither has an editor.
function CheckableRow({
	id,
	number,
	checked,
	onToggle,
	onExecute,
	onStop,
	execRun,
	dragDisabled,
	onGripPointerDown,
	onGripPointerUp,
	children,
}: CheckableRowProps) {
	return (
		<div
			className={cn(
				'flex overflow-hidden rounded-md border border-dashed bg-muted/25 text-sm',
				checked && DONE_SURFACE,
			)}
		>
			<RowGutter
				id={id}
				number={number}
				checked={checked}
				onToggle={onToggle}
				dragDisabled={dragDisabled}
				onGripPointerDown={onGripPointerDown}
				onGripPointerUp={onGripPointerUp}
			/>
			<div className="flex min-w-0 flex-1 flex-col">
				<div className="flex h-9 shrink-0 items-center pl-3">
					<div className="min-w-0 flex-1" />
					<RunControls execRun={execRun} onStop={onStop} />
					<ExecuteButton onClick={onExecute} />
				</div>
				<RowBody id={id} checked={checked}>
					{children}
				</RowBody>
			</div>
		</div>
	)
}

interface ExecutableActionRowProps {
	id: string
	number: string
	checked: boolean
	comment: string
	header: ReactNode
	onToggle: (key: string, nextChecked: boolean) => void
	onComment: () => void
	onEdit: () => void
	onExecute: () => void
	onStop: () => void
	onViewLogs?: () => void
	execRun?: ActionExecRun
	executorType?: ExecutorType | null
	executeDisabled?: boolean
	executeDisabledReason?: string
	dragDisabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
	children: ReactNode
}

function ExecutableActionRow({
	id,
	number,
	checked,
	comment,
	header,
	onToggle,
	onComment,
	onEdit,
	onExecute,
	onStop,
	onViewLogs,
	execRun,
	executorType = null,
	executeDisabled = false,
	executeDisabledReason,
	dragDisabled,
	onGripPointerDown,
	onGripPointerUp,
	children,
}: ExecutableActionRowProps) {
	const hasComment = comment.trim().length > 0

	return (
		<div
			className={cn(
				'flex overflow-hidden rounded-md border bg-muted/60 text-sm',
				checked && DONE_SURFACE,
			)}
		>
			<RowGutter
				id={id}
				number={number}
				checked={checked}
				onToggle={onToggle}
				dragDisabled={dragDisabled}
				onGripPointerDown={onGripPointerDown}
				onGripPointerUp={onGripPointerUp}
			/>
			<div className="flex min-w-0 flex-1 flex-col">
				<div className="flex h-9 shrink-0 items-center pl-3">
					<div className="flex min-w-0 flex-1 items-center pr-2">{header}</div>
					<RunControls
						execRun={execRun}
						onStop={onStop}
						onViewLogs={onViewLogs}
						executorType={executorType}
					/>
					<HeaderButton
						label="Edit action"
						tooltip="Edit action"
						onClick={onEdit}
					>
						<Pencil className="h-4 w-4" />
					</HeaderButton>
					<HeaderButton
						label={hasComment ? 'Edit comment' : 'Add comment'}
						tooltip={hasComment ? 'Edit comment' : 'Add comment'}
						onClick={onComment}
						active={hasComment}
					>
						<MessageSquare
							className={cn('h-4 w-4', hasComment && 'fill-current')}
						/>
					</HeaderButton>
					<ExecuteButton
						onClick={onExecute}
						disabled={executeDisabled}
						disabledReason={executeDisabledReason}
					/>
				</div>
				<RowBody id={id} checked={checked}>
					{children}
				</RowBody>
			</div>
		</div>
	)
}

interface SortableListItemProps {
	scope: ActionPlanScope
	stage: number
	index: number
	dragState: DragState | null
	dropTarget: number | null
	dragEnabled: boolean
	onDragStart: (e: DragEvent<HTMLLIElement>) => void
	onDragEnd: () => void
	onDragOver: (e: DragEvent<HTMLLIElement>) => void
	onDrop: (e: DragEvent<HTMLLIElement>) => void
	children: ReactNode
}

function SortableListItem({
	scope,
	stage,
	index,
	dragState,
	dropTarget,
	dragEnabled,
	onDragStart,
	onDragEnd,
	onDragOver,
	onDrop,
	children,
}: SortableListItemProps) {
	const isDragging =
		dragState?.scope === scope &&
		dragState.stage === stage &&
		dragState.index === index
	const isDropTarget =
		dropTarget === index &&
		dragState !== null &&
		dragState.scope === scope &&
		dragState.stage === stage &&
		dragState.index !== index

	return (
		<li
			draggable={dragEnabled}
			onDragStart={onDragStart}
			onDragEnd={onDragEnd}
			onDragOver={onDragOver}
			onDrop={onDrop}
			className={cn(
				isDragging && 'opacity-50',
				isDropTarget && 'rounded-md ring-2 ring-primary ring-offset-1',
			)}
		>
			{children}
		</li>
	)
}

export function ActionPlanView({
	plan,
	checked,
	comments,
	onToggle,
	onComment,
	onEdit,
	onExecute,
	onStop,
	onViewLogs,
	execRuns,
	executorType = null,
	onReorder,
	reordering = false,
	executorDisabled = false,
	fanoutRun = null,
	fanoutRunning = false,
	stageRun = null,
	stageRunBusy = false,
	canReplan = false,
	onOpenStageChat,
	onReplanStage,
	onReplanRollback,
	onRunStage,
	onStopStageRun,
}: ActionPlanViewProps) {
	const gripEnabledRef = useRef(false)
	const [gripActive, setGripActive] = useState(false)
	const [dragState, setDragState] = useState<DragState | null>(null)
	const [dropTarget, setDropTarget] = useState<number | null>(null)

	const dragDisabled = reordering
	const dragEnabled = gripActive && !dragDisabled

	const resetGrip = useCallback(() => {
		gripEnabledRef.current = false
		setGripActive(false)
	}, [])

	const handleGripPointerDown = useCallback(() => {
		if (!dragDisabled) {
			gripEnabledRef.current = true
			setGripActive(true)
		}
	}, [dragDisabled])

	const handleGripPointerUp = useCallback(() => {
		resetGrip()
	}, [resetGrip])

	const clearDrag = useCallback(() => {
		resetGrip()
		setDragState(null)
		setDropTarget(null)
	}, [resetGrip])

	useEffect(() => {
		if (!gripActive) return

		const handleRelease = () => {
			resetGrip()
		}

		window.addEventListener('pointerup', handleRelease)
		window.addEventListener('dragend', handleRelease)

		return () => {
			window.removeEventListener('pointerup', handleRelease)
			window.removeEventListener('dragend', handleRelease)
		}
	}, [gripActive, resetGrip])

	const handleDragStart = useCallback(
		(
			e: DragEvent<HTMLLIElement>,
			scope: ActionPlanScope,
			stage: number,
			index: number,
		) => {
			if (!gripEnabledRef.current || dragDisabled) {
				e.preventDefault()
				return
			}
			setDragState({ scope, stage, index })
			e.dataTransfer.effectAllowed = 'move'
			e.dataTransfer.setData('text/plain', `${scope}:${stage}:${index}`)
		},
		[dragDisabled],
	)

	const handleDragOver = useCallback(
		(
			e: DragEvent<HTMLLIElement>,
			scope: ActionPlanScope,
			stage: number,
			index: number,
		) => {
			if (
				!dragState ||
				dragState.scope !== scope ||
				dragState.stage !== stage
			) {
				e.dataTransfer.dropEffect = 'none'
				return
			}
			e.preventDefault()
			e.dataTransfer.dropEffect = 'move'
			setDropTarget(index)
		},
		[dragState],
	)

	const handleDrop = useCallback(
		(
			e: DragEvent<HTMLLIElement>,
			scope: ActionPlanScope,
			stage: number,
			to: number,
		) => {
			e.preventDefault()
			if (
				!dragState ||
				dragState.scope !== scope ||
				dragState.stage !== stage
			) {
				clearDrag()
				return
			}
			const from = dragState.index
			if (from !== to) {
				onReorder(scope, stage, from, to)
			}
			clearDrag()
		},
		[dragState, onReorder, clearDrag],
	)

	const checkedSet = new Set(checked)
	const stages = plan?.stages ?? []
	const runningElsewhere = (scope: StageRunScope, idx: number) =>
		stageRun?.status === 'running' && !isStageRunFor(stageRun, scope, idx)
			? stageRun
			: null

	// Only the run of this stage explains itself here; any other stage's stop
	// reason belongs to that stage.
	const stopNote = (scope: StageRunScope, idx: number) => {
		if (!stageRun || stageRun.status !== 'stopped' || stageRun.scope !== scope) {
			return null
		}
		if (scope === 'stage' && stageRun.stage !== idx) return null
		const at = stageRun.current ? actionPlanNumberForKey(stageRun.current) : ''
		return (
			<p className="rounded-md border border-dashed px-3 py-2 text-sm text-muted-foreground">
				Run stopped{at ? ` at ${at}` : ''}
				{stageRun.reason ? `: ${stageRun.reason}` : ''}
			</p>
		)
	}

	const rollbackFanout = findFanoutRollback(fanoutRun)
	const rollbackKeys = plan ? rollbackItemKeys(plan) : []
	const rollbackRunActive = isStageRunFor(stageRun, 'rollback', 0)
	const showRollback = (plan?.rollback.length ?? 0) > 0 || Boolean(rollbackFanout)

	return (
		<div className="space-y-4">
			{mergePlanStages(plan, fanoutRun).map((entry) => {
				const stageIdx = entry.index
				const stage = stageIdx === null ? null : stages[stageIdx]
				const keys = plan && stageIdx !== null ? stageItemKeys(plan, stageIdx) : []
				const runActive = stageIdx !== null && isStageRunFor(stageRun, 'stage', stageIdx)
				const dialogId = entry.fanout?.dialogId
				const codeBlocked =
					executorDisabled &&
					(stage?.steps ?? []).some(
						(step, i) => isCodeStep(step) && !checkedSet.has(`s${stageIdx}.step${i}`),
					)

				return (
					<StageBlock
						key={stageIdx === null ? `pending:${entry.title}` : `stage-${stageIdx}`}
						heading={
							stageIdx === null
								? entry.title
								: `${actionPlanStageNumber(stageIdx)}. ${entry.title}`
						}
						badge={
							stage && stageIdx !== null ? (
								<StageImpactBadge
									steps={stage.steps}
									numberOf={(i) => actionPlanItemNumber('step', stageIdx, i)}
								/>
							) : null
						}
						status={deriveStageStatus({
							keys,
							checked: checkedSet,
							execRuns,
							fanout: entry.fanout,
							fanoutRunning,
							runActive,
						})}
						complete={isItemsComplete(keys, checkedSet)}
						onOpenChat={
							dialogId && onOpenStageChat ? () => onOpenStageChat(dialogId) : undefined
						}
						onReplan={
							canReplan && onReplanStage ? () => onReplanStage(entry.title) : undefined
						}
						runActive={runActive}
						runDisabledReason={runAllDisabledReason({
							keys,
							checked: checkedSet,
							execRuns,
							fanoutRunning,
							otherRun: stageIdx === null ? null : runningElsewhere('stage', stageIdx),
							codeBlocked,
						})}
						runBusy={stageRunBusy}
						onRun={() => stageIdx !== null && onRunStage?.('stage', stageIdx)}
						onStop={() => onStopStageRun?.()}
						note={stageIdx === null ? null : stopNote('stage', stageIdx)}
					>
						{stage &&
						stageIdx !== null &&
						(stage.steps.length > 0 || stage.checks.length > 0) ? (
							<>
								{stage.steps.length > 0 ? (
									<ul className="space-y-2">
										{stage.steps.map((step, stepIdx) => {
											const key = `s${stageIdx}.step${stepIdx}`
											return (
												<SortableListItem
													key={key}
													scope="steps"
													stage={stageIdx}
													index={stepIdx}
													dragState={dragState}
													dropTarget={dropTarget}
													dragEnabled={dragEnabled}
													onDragStart={(e) =>
														handleDragStart(e, 'steps', stageIdx, stepIdx)
													}
													onDragEnd={clearDrag}
													onDragOver={(e) =>
														handleDragOver(e, 'steps', stageIdx, stepIdx)
													}
													onDrop={(e) =>
														handleDrop(e, 'steps', stageIdx, stepIdx)
													}
												>
													<ExecutableActionRow
														id={key}
														number={actionPlanItemNumber(
															'step',
															stageIdx,
															stepIdx,
														)}
														checked={checkedSet.has(key)}
														comment={comments[key] ?? ''}
														header={<ActionRowHeader step={step} />}
														onToggle={onToggle}
														onComment={() => onComment(key)}
														onEdit={() => onEdit(key)}
														onExecute={() => onExecute(key)}
														onStop={onStop}
														onViewLogs={
															onViewLogs ? () => onViewLogs(key) : undefined
														}
														execRun={execRuns[key]}
														executorType={executorType}
														executeDisabled={
															executorDisabled && isCodeStep(step)
														}
														executeDisabledReason={EXECUTOR_DISABLED_REASON}
														dragDisabled={dragDisabled}
														onGripPointerDown={handleGripPointerDown}
														onGripPointerUp={handleGripPointerUp}
													>
														<ActionBody step={step} />
													</ExecutableActionRow>
												</SortableListItem>
											)
										})}
									</ul>
								) : null}

								{stage.checks.length > 0 ? (
									<div className="space-y-2">
										<p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
											Checks
										</p>
										<ul className="space-y-2">
											{stage.checks.map((check, checkIdx) => {
												const key = `s${stageIdx}.check${checkIdx}`
												return (
													<SortableListItem
														key={key}
														scope="checks"
														stage={stageIdx}
														index={checkIdx}
														dragState={dragState}
														dropTarget={dropTarget}
														dragEnabled={dragEnabled}
														onDragStart={(e) =>
															handleDragStart(e, 'checks', stageIdx, checkIdx)
														}
														onDragEnd={clearDrag}
														onDragOver={(e) =>
															handleDragOver(e, 'checks', stageIdx, checkIdx)
														}
														onDrop={(e) =>
															handleDrop(e, 'checks', stageIdx, checkIdx)
														}
													>
														<CheckableRow
															id={key}
															number={actionPlanItemNumber(
																'check',
																stageIdx,
																checkIdx,
															)}
															checked={checkedSet.has(key)}
															onToggle={onToggle}
															onExecute={() => onExecute(key)}
															onStop={onStop}
															execRun={execRuns[key]}
															dragDisabled={dragDisabled}
															onGripPointerDown={handleGripPointerDown}
															onGripPointerUp={handleGripPointerUp}
														>
															<MarkdownMessage content={check.check} />
															{check.expectation ? (
																<div className="mt-1 text-xs text-muted-foreground">
																	<MarkdownMessage content={check.expectation} />
																</div>
															) : null}
														</CheckableRow>
													</SortableListItem>
												)
											})}
										</ul>
									</div>
								) : null}
							</>
						) : null}
					</StageBlock>
				)
			})}

			{showRollback ? (
				<StageBlock
					heading="Rollback"
					badge={
						plan ? (
							<StageImpactBadge
								steps={plan.rollback}
								numberOf={(i) => actionPlanItemNumber('rollback', 0, i)}
							/>
						) : null
					}
					// Rollback is the plan's escape hatch, not part of reading it:
					// collapsed until the operator goes looking for it.
					defaultExpanded={false}
					status={deriveStageStatus({
						keys: rollbackKeys,
						checked: checkedSet,
						execRuns,
						fanout: rollbackFanout,
						fanoutRunning,
						runActive: rollbackRunActive,
					})}
					complete={isItemsComplete(rollbackKeys, checkedSet)}
					onOpenChat={
						rollbackFanout?.dialogId && onOpenStageChat
							? () => onOpenStageChat(rollbackFanout.dialogId as string)
							: undefined
					}
					onReplan={canReplan && onReplanRollback ? onReplanRollback : undefined}
					runActive={rollbackRunActive}
					runDisabledReason={runAllDisabledReason({
						keys: rollbackKeys,
						checked: checkedSet,
						execRuns,
						fanoutRunning,
						otherRun: runningElsewhere('rollback', 0),
						codeBlocked:
							executorDisabled &&
							(plan?.rollback ?? []).some(
								(step, i) => isCodeStep(step) && !checkedSet.has(`rollback.${i}`),
							),
					})}
					runBusy={stageRunBusy}
					onRun={() => onRunStage?.('rollback', 0)}
					onStop={() => onStopStageRun?.()}
					note={stopNote('rollback', 0)}
				>
					{plan && plan.rollback.length > 0 ? (
					<ul className="space-y-2">
						{plan.rollback.map((step, idx) => {
							const key = `rollback.${idx}`
							return (
								<li key={key}>
									<ExecutableActionRow
										id={key}
										number={actionPlanItemNumber('rollback', 0, idx)}
										checked={checkedSet.has(key)}
										comment={comments[key] ?? ''}
										header={<ActionRowHeader step={step} />}
										onToggle={onToggle}
										onComment={() => onComment(key)}
										onEdit={() => onEdit(key)}
										onExecute={() => onExecute(key)}
										onStop={onStop}
										onViewLogs={
											onViewLogs ? () => onViewLogs(key) : undefined
										}
										execRun={execRuns[key]}
										executorType={executorType}
										executeDisabled={
											executorDisabled && isCodeStep(step)
										}
										executeDisabledReason={EXECUTOR_DISABLED_REASON}
										dragDisabled
										onGripPointerDown={() => {}}
										onGripPointerUp={() => {}}
									>
										<ActionBody step={step} />
									</ExecutableActionRow>
								</li>
							)
						})}
					</ul>
					) : null}
				</StageBlock>
			) : null}
		</div>
	)
}
