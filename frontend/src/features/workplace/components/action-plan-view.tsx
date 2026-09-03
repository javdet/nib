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
	ChevronDown,
	ChevronUp,
	FolderGit2,
	GripVertical,
	Loader2,
	MessageSquare,
	Pencil,
	Play,
	RotateCcw,
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
} from '@/features/dialogs/api/dialogs'
import type { ActionPlanScope } from '@/features/dialogs/api/dialogs'
import { actionTypeIcon } from '../lib/action-type-icon'
import {
	actionPlanItemNumber,
	actionPlanStageNumber,
} from '../lib/action-plan-number'

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

// Done is an opaque emerald surface rather than a translucent green wash, so a
// finished row reads as a settled state instead of a film laid over the card.
// Redefining --border retints every divider inside the row at once: the base
// layer resolves every border-color through it.
const DONE_SURFACE =
	'border-success-border bg-success-surface [--border:var(--success-border)]'

interface ActionPlanViewProps {
	plan: ActionPlan
	checked: string[]
	comments: Record<string, string>
	onToggle: (key: string, nextChecked: boolean) => void
	onComment: (key: string) => void
	onEdit: (key: string) => void
	onExecute: (step: ActionStep, key: string) => void
	// onRestart abandons whatever is running on a row and starts a fresh
	// sub-agent. Only offered once a row has a run to restart.
	onRestart: (key: string) => void
	execRuns: ActionExecRuns
	onReorder: (scope: ActionPlanScope, stage: number, from: number, to: number) => void
	reordering?: boolean
	// executorDisabled mirrors the "disabled" executor type: code actions have
	// nowhere to run, so their Execute button is turned off.
	executorDisabled?: boolean
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
					'border-b font-mono text-xs tabular-nums',
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

interface HeaderButtonProps {
	label: string
	tooltip: ReactNode
	onClick: () => void
	disabled?: boolean
	active?: boolean
	children: ReactNode
}

function HeaderButton({
	label,
	tooltip,
	onClick,
	disabled = false,
	active = false,
	children,
}: HeaderButtonProps) {
	return (
		<Tooltip>
			{/* The button is wrapped so the tooltip still explains why it is off:
			    a natively disabled button swallows its own pointer events. */}
			<TooltipTrigger asChild>
				<span className="flex h-9 w-9 shrink-0 self-stretch border-l">
					<button
						type="button"
						onClick={onClick}
						disabled={disabled}
						className={cn(
							'flex flex-1 items-center justify-center transition-colors',
							'duration-[var(--dur-fast)] focus-ring-inset',
							disabled && 'cursor-not-allowed text-muted-foreground/40',
							!disabled && 'cursor-pointer hover:bg-foreground/[0.07]',
							!disabled &&
								(active
									? 'text-primary hover:text-primary'
									: 'text-muted-foreground hover:text-foreground'),
						)}
						aria-label={label}
					>
						{children}
					</button>
				</span>
			</TooltipTrigger>
			<TooltipContent>{tooltip}</TooltipContent>
		</Tooltip>
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

function RepositoryBadge({ step }: { step: ActionStep }) {
	const repository = step.repository?.trim()
	if (!isCodeStep(step) || !repository) return null

	return (
		<Badge
			variant="outline"
			className="max-w-full font-normal"
			title={`repository: ${repository}`}
		>
			<FolderGit2 className="size-3.5 shrink-0" aria-hidden />
			<span className="truncate">{repository}</span>
		</Badge>
	)
}

// The type icon sits beside the narrative in a flex row so every line of text
// shares the same left edge instead of wrapping around a float.
function ActionTypeTile({ type }: { type: string }) {
	const TypeIcon = actionTypeIcon(type)

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span
					tabIndex={0}
					aria-label={type}
					className={cn(
						'flex size-10 shrink-0 items-center justify-center',
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

	const narrative = (
		<div className="min-w-0 flex-1">
			<MarkdownMessage content={step.action} />
			{isCodeStep(step) ? (
				<div className="mt-2 text-xs">
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
				<div className="mt-2 flex items-start gap-1 rounded-md border bg-background/70 py-1 pl-2 pr-1">
					<code className="min-w-0 flex-1 overflow-x-auto whitespace-pre py-1 font-mono text-xs leading-relaxed">
						{command}
					</code>
					<CopyButton text={command} label="Copy command" />
				</div>
			) : null}
		</div>
	)

	if (!type) return narrative

	return (
		<div className="flex items-start gap-3">
			<ActionTypeTile type={type} />
			{narrative}
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

interface CheckableRowProps {
	id: string
	number: string
	checked: boolean
	onToggle: (key: string, nextChecked: boolean) => void
	dragDisabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
	children: ReactNode
}

function CheckableRow({
	id,
	number,
	checked,
	onToggle,
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
			<div className="min-w-0 flex-1">
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
	onRestart: () => void
	execRun?: ActionExecRun
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
	onRestart,
	execRun,
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
				<div className="flex h-9 shrink-0 items-center border-b pl-3">
					<div className="flex min-w-0 flex-1 items-center pr-2">{header}</div>
					{execRun && (
						<Tooltip>
							<TooltipTrigger asChild>
								<span className="flex h-9 w-9 shrink-0 items-center justify-center">
									<ExecStatusDot run={execRun} />
								</span>
							</TooltipTrigger>
							<TooltipContent>{execRunTooltip(execRun)}</TooltipContent>
						</Tooltip>
					)}
					{execRun && execRun.status !== 'running' && (
						<HeaderButton
							label="Run action again"
							tooltip="Run again with a new sub-agent"
							onClick={onRestart}
						>
							<RotateCcw className="h-4 w-4" />
						</HeaderButton>
					)}
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
					<HeaderButton
						label="Execute action"
						tooltip={
							executeDisabled && executeDisabledReason
								? executeDisabledReason
								: 'Execute action'
						}
						onClick={onExecute}
						disabled={executeDisabled}
					>
						<span className="flex size-6 items-center justify-center rounded-full border border-current/40">
							<Play className="ml-0.5 size-3 fill-current" />
						</span>
					</HeaderButton>
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
	onRestart,
	execRuns,
	onReorder,
	reordering = false,
	executorDisabled = false,
}: ActionPlanViewProps) {
	const checkedSet = new Set(checked)
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

	return (
		<div className="space-y-6">
			{plan.stages.map((stage, stageIdx) => (
				<section key={`stage-${stageIdx}`} className="space-y-3">
					<h3 className="text-lg font-semibold">
						{actionPlanStageNumber(stageIdx)}. {stage.title}
					</h3>

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
											header={<RepositoryBadge step={step} />}
											onToggle={onToggle}
											onComment={() => onComment(key)}
											onEdit={() => onEdit(key)}
											onExecute={() => onExecute(step, key)}
											onRestart={() => onRestart(key)}
											execRun={execRuns[key]}
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
				</section>
			))}

			{plan.rollback.length > 0 ? (
				<section className="space-y-3">
					<h3 className="text-lg font-semibold">Rollback</h3>
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
										header={<RepositoryBadge step={step} />}
										onToggle={onToggle}
										onComment={() => onComment(key)}
										onEdit={() => onEdit(key)}
										onExecute={() => onExecute(step, key)}
										onRestart={() => onRestart(key)}
										execRun={execRuns[key]}
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
				</section>
			) : null}
		</div>
	)
}
