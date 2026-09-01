import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type DragEvent,
	type ReactNode,
} from 'react'
import { GripVertical, MessageSquare, Pencil, Play } from 'lucide-react'
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
import type { ActionPlan, ActionStep } from '@/features/dialogs/api/dialogs'
import type { ActionPlanScope } from '@/features/dialogs/api/dialogs'

const EXECUTOR_DISABLED_REASON =
	'Executor is disabled — set its type in executor settings to run code actions'

interface ActionPlanViewProps {
	plan: ActionPlan
	checked: string[]
	comments: Record<string, string>
	onToggle: (key: string, nextChecked: boolean) => void
	onComment: (key: string) => void
	onEdit: (key: string) => void
	onExecute: (step: ActionStep, key: string) => void
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
				'flex w-8 shrink-0 select-none items-center justify-center self-stretch border-r',
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

interface CheckableRowProps {
	id: string
	checked: boolean
	onToggle: (key: string, nextChecked: boolean) => void
	dragDisabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
	children: ReactNode
}

function CheckableRow({
	id,
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
				checked && 'border-green-500/40 bg-green-500/10',
			)}
		>
			<DragHandle
				disabled={dragDisabled}
				onGripPointerDown={onGripPointerDown}
				onGripPointerUp={onGripPointerUp}
			/>
			<div className="flex min-w-0 flex-1 select-text items-start gap-3 px-3 py-2">
				<Checkbox
					checked={checked}
					onCheckedChange={(value) => onToggle(id, value === true)}
					aria-labelledby={`${id}-label`}
					className="mt-0.5 cursor-pointer"
				/>
				<div id={`${id}-label`} className="min-w-0 flex-1">
					{children}
				</div>
			</div>
		</div>
	)
}

function ActionLabels({ step }: { step: ActionStep }) {
	const type = step.type?.trim()
	const repository = step.repository?.trim()
	const showRepository = type?.toLowerCase() === 'code' && !!repository

	if (!type && !showRepository) return null

	return (
		<div className="mb-1 flex flex-wrap items-center gap-1.5">
			{type ? <Badge variant="secondary">type: {type}</Badge> : null}
			{showRepository ? (
				<Badge variant="outline">repository: {repository}</Badge>
			) : null}
		</div>
	)
}

// ActionBody renders the step narrative and, for shell and curl steps, the
// commands themselves as a copyable block.
function ActionBody({ step }: { step: ActionStep }) {
	const command = step.command?.trim()
	const isCode = step.type?.trim().toLowerCase() === 'code'
	const prURL = step.pr_url?.trim()

	return (
		<>
			<ActionLabels step={step} />
			<MarkdownMessage content={step.action} />
			{isCode ? (
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
		</>
	)
}

interface ExecutableActionRowProps {
	id: string
	checked: boolean
	comment: string
	onToggle: (key: string, nextChecked: boolean) => void
	onComment: () => void
	onEdit: () => void
	onExecute: () => void
	executeDisabled?: boolean
	executeDisabledReason?: string
	dragDisabled?: boolean
	onGripPointerDown: () => void
	onGripPointerUp: () => void
	children: ReactNode
}

function ExecutableActionRow({
	id,
	checked,
	comment,
	onToggle,
	onComment,
	onEdit,
	onExecute,
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
				checked && 'border-green-500/40 bg-green-500/10',
			)}
		>
			<DragHandle
				disabled={dragDisabled}
				onGripPointerDown={onGripPointerDown}
				onGripPointerUp={onGripPointerUp}
			/>
			<div className="flex min-w-0 flex-1 select-text items-start gap-3 px-3 py-2">
				<Checkbox
					checked={checked}
					onCheckedChange={(value) => onToggle(id, value === true)}
					aria-labelledby={`${id}-label`}
					className="mt-0.5 cursor-pointer"
				/>
				<div id={`${id}-label`} className="min-w-0 flex-1">
					{children}
				</div>
			</div>
			<div className="flex w-12 shrink-0 flex-col self-stretch border-l">
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							onClick={onEdit}
							className={cn(
								'flex flex-1 cursor-pointer items-center justify-center',
								'text-muted-foreground transition-colors focus-ring-inset',
								'duration-[var(--dur-fast)] hover:bg-foreground/[0.07]',
								'hover:text-foreground',
							)}
							aria-label="Edit action"
						>
							<Pencil className="h-4 w-4" />
						</button>
					</TooltipTrigger>
					<TooltipContent>Edit action</TooltipContent>
				</Tooltip>
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							onClick={onComment}
							className={cn(
								'flex flex-1 cursor-pointer items-center justify-center border-t',
								'transition-colors duration-[var(--dur-fast)] focus-ring-inset',
								'hover:bg-foreground/[0.07]',
								hasComment
									? 'text-primary hover:text-primary'
									: 'text-muted-foreground hover:text-foreground',
							)}
							aria-label={hasComment ? 'Edit comment' : 'Add comment'}
						>
							<MessageSquare
								className={cn('h-4 w-4', hasComment && 'fill-current')}
							/>
						</button>
					</TooltipTrigger>
					<TooltipContent>
						{hasComment ? 'Edit comment' : 'Add comment'}
					</TooltipContent>
				</Tooltip>
			</div>
			<Tooltip>
				{/* The button is wrapped so the tooltip still explains why it is off:
				    a natively disabled button swallows its own pointer events. */}
				<TooltipTrigger asChild>
					<span className="flex w-12 shrink-0 self-stretch border-l">
						<button
							type="button"
							onClick={onExecute}
							disabled={executeDisabled}
							className={cn(
								'flex flex-1 items-center justify-center transition-colors',
								'duration-[var(--dur-fast)] focus-ring-inset',
								executeDisabled
									? 'cursor-not-allowed text-muted-foreground/40'
									: 'cursor-pointer text-muted-foreground hover:bg-foreground/[0.07] hover:text-foreground',
							)}
							aria-label="Execute action"
						>
							<span className="flex h-7 w-7 items-center justify-center rounded-full border border-current/40">
								<Play className="ml-0.5 h-3 w-3 fill-current" />
							</span>
						</button>
					</span>
				</TooltipTrigger>
				<TooltipContent>
					{executeDisabled && executeDisabledReason
						? executeDisabledReason
						: 'Execute action'}
				</TooltipContent>
			</Tooltip>
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
					<div>
						<h3 className="text-sm font-semibold">{stage.title}</h3>
						{stage.description ? (
							<p className="mt-1 text-xs text-muted-foreground">
								{stage.description}
							</p>
						) : null}
					</div>

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
											checked={checkedSet.has(key)}
											comment={comments[key] ?? ''}
											onToggle={onToggle}
											onComment={() => onComment(key)}
											onEdit={() => onEdit(key)}
											onExecute={() => onExecute(step, key)}
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
					<h3 className="text-sm font-semibold">Rollback</h3>
					<ul className="space-y-2">
						{plan.rollback.map((step, idx) => {
							const key = `rollback.${idx}`
							return (
								<li key={key}>
									<ExecutableActionRow
										id={key}
										checked={checkedSet.has(key)}
										comment={comments[key] ?? ''}
										onToggle={onToggle}
										onComment={() => onComment(key)}
										onEdit={() => onEdit(key)}
										onExecute={() => onExecute(step, key)}
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
