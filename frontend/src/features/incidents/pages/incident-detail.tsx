import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { ArrowLeft, CheckCheck, FileText, GitPullRequestArrow } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
import { IconButton } from '@/components/ui/icon-button'
import { cn } from '@/lib/utils'
import { downloadTextFile } from '@/lib/download'
import { extractErrorMessage } from '@/lib/api-client'
import { getDialog, type Dialog } from '@/features/dialogs/api/dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { useMode } from '@/features/modes/mode-context'
import {
	closeIncident,
	createPlanFromIncident,
	stopInvestigation,
	type PostmortemAction,
	type PostmortemItemFields,
	type PostmortemListName,
	type PostmortemScalarField,
} from '../api/incidents'
import { isActionSelectable, pruneSelection } from '../lib/action-selection'
import { CloseIncidentDialog } from '../components/close-incident-dialog'
import { EditableListCard, type ListFieldSpec } from '../components/editable-list-card'
import { FindingsCard } from '../components/findings-card'
import { InvestigationCard } from '../components/investigation-card'
import { EditableMarkdownSection } from '../components/editable-markdown-section'
import { PostmortemHeader } from '../components/postmortem-header'
import { useInvestigation } from '../hooks/use-investigation'
import { usePostmortem } from '../hooks/use-postmortem'
import { formatIncidentTime } from '../lib/incident-time'
import {
	buildPostmortemMarkdown,
	postmortemMarkdownFileName,
} from '../lib/postmortem-markdown'
import { actionStatusLabel } from '../lib/postmortem-labels'

const ACTION_STATUS_OPTIONS = [
	{ value: 'todo', label: 'To do' },
	{ value: 'done', label: 'Done' },
	{ value: 'dropped', label: 'Dropped' },
]

const ROOT_CAUSE_FIELDS: ListFieldSpec[] = [
	{ key: 'text', label: 'Root cause', kind: 'textarea', required: true },
]

const IMPACTED_FIELDS: ListFieldSpec[] = [
	{ key: 'name', label: 'System or area', required: true, span: 2 },
	{ key: 'environment', label: 'Environment', span: 2 },
	{ key: 'severity', label: 'Severity', placeholder: 'e.g. degraded, down', span: 2 },
	{ key: 'description', label: 'How it was affected', kind: 'textarea' },
]

const TIMELINE_FIELDS: ListFieldSpec[] = [
	{ key: 'at', label: 'Time', kind: 'datetime', span: 2 },
	{ key: 'source', label: 'Source', placeholder: 'alert, system, tool', span: 4 },
	{ key: 'event', label: 'Event', kind: 'textarea', required: true },
]

const ACTION_FIELDS: ListFieldSpec[] = [
	{ key: 'text', label: 'Action', kind: 'textarea', required: true, span: 4 },
	{ key: 'status', label: 'Status', kind: 'select', options: ACTION_STATUS_OPTIONS, span: 2 },
]

function ActionText({
	action,
	onOpenPlan,
}: {
	action: PostmortemAction
	onOpenPlan: (planId: string) => void
}) {
	const pill =
		'shrink-0 rounded-full border border-border/60 px-2 text-[10px] uppercase tracking-wider text-muted-foreground'
	return (
		<div className="flex items-start gap-2">
			<span
				className={cn(
					'whitespace-pre-wrap',
					(action.status === 'done' || action.status === 'dropped') &&
						'text-muted-foreground line-through',
				)}
			>
				{action.text}
			</span>
			{action.planId ? (
				<button
					type="button"
					className={cn(pill, 'hover:bg-secondary hover:text-foreground')}
					onClick={() => onOpenPlan(action.planId!)}
				>
					{actionStatusLabel(action.status)} ↗
				</button>
			) : (
				action.status !== 'todo' && (
					<span className={pill}>{actionStatusLabel(action.status)}</span>
				)
			)}
		</div>
	)
}

export function IncidentDetail() {
	const { id } = useParams<{ id: string }>()
	const navigate = useNavigate()
	const { setActiveDialogId, dialogsVersion, bumpDialogsVersion, enqueuePendingMessage } =
		useDialog()
	const { selectMode } = useMode()
	const [dialog, setDialog] = useState<Dialog | null>(null)
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)
	const { run, refresh: refreshRun, handleActivity } = useInvestigation(id)
	const { postmortem, error: postmortemError, edit } = usePostmortem(id, handleActivity)
	const [selected, setSelected] = useState<Set<string>>(() => new Set())
	const [creatingPlan, setCreatingPlan] = useState(false)
	const [planError, setPlanError] = useState<string | null>(null)
	const [closeOpen, setCloseOpen] = useState(false)

	useEffect(() => {
		if (postmortem) setSelected((prev) => pruneSelection(prev, postmortem))
	}, [postmortem])

	const loadDetail = useCallback(async (dialogId: string) => {
		setLoading(true)
		setError(null)
		try {
			const dialogData = await getDialog(dialogId)
			setDialog(dialogData)
			selectMode(dialogData.mode)
		} catch (err) {
			setDialog(null)
			setError(
				err instanceof Error ? err.message : 'Failed to load incident',
			)
		} finally {
			setLoading(false)
		}
	}, [selectMode])

	useEffect(() => {
		if (!id) return
		setActiveDialogId(id)
		void loadDetail(id)
	}, [id, setActiveDialogId, loadDetail, dialogsVersion])

	const handleBack = useCallback(() => {
		void navigate('/incidents')
	}, [navigate])

	const handleDownload = useCallback(() => {
		if (!postmortem) return
		const title = dialog?.title ?? ''
		downloadTextFile(
			postmortemMarkdownFileName(title),
			buildPostmortemMarkdown(title, postmortem),
		)
	}, [dialog?.title, postmortem])

	const handleOpenPlan = useCallback(
		(planId: string) => {
			selectMode('main')
			setActiveDialogId(planId)
			void navigate(`/workplace/${planId}`)
		},
		[navigate, selectMode, setActiveDialogId],
	)

	const handleToggleAction = useCallback((actionId: string, checked: boolean) => {
		setSelected((prev) => {
			const next = new Set(prev)
			if (checked) next.add(actionId)
			else next.delete(actionId)
			return next
		})
	}, [])

	const handleCreatePlan = useCallback(async () => {
		if (!id || selected.size === 0) return
		setCreatingPlan(true)
		setPlanError(null)
		try {
			const plan = await createPlanFromIncident(id, [...selected])
			setSelected(new Set())
			// The orchestrator's prompt is frozen on its first message, so the
			// mode is settled before the chat panel sends it for us.
			selectMode('main')
			enqueuePendingMessage({ dialogId: plan.planId, text: plan.message })
			setActiveDialogId(plan.planId)
			bumpDialogsVersion()
			void navigate(`/workplace/${plan.planId}`)
		} catch (err) {
			setPlanError(extractErrorMessage(err))
		} finally {
			setCreatingPlan(false)
		}
	}, [
		id,
		selected,
		selectMode,
		enqueuePendingMessage,
		setActiveDialogId,
		bumpDialogsVersion,
		navigate,
	])

	const handleStopRun = useCallback(async () => {
		if (!id) return
		await stopInvestigation(id)
		refreshRun()
	}, [id, refreshRun])

	const handleClose = useCallback(
		async (resolvedAt: string) => {
			if (!id) return
			await closeIncident(id, resolvedAt)
			refreshRun()
		},
		[id, refreshRun],
	)

	const runActive = run?.status === 'running'
	const closed = postmortem?.status === 'closed'

	const saveScalar = (field: PostmortemScalarField) => (value: string) =>
		edit({ set: { [field]: value } })
	const unlockScalar = (field: PostmortemScalarField) => () =>
		edit({ unlock: [field] })
	const addItem = (list: PostmortemListName) => (fields: PostmortemItemFields) =>
		edit({ ops: [{ list, op: 'add', fields }] })
	const updateItem = (list: PostmortemListName) => (itemId: string, fields: PostmortemItemFields) =>
		edit({ ops: [{ list, op: 'update', id: itemId, fields }] })
	const deleteItem = (list: PostmortemListName) => (itemId: string) =>
		edit({ ops: [{ list, op: 'delete', id: itemId }] })

	if (!id) {
		return (
			<div className="text-sm text-muted-foreground">
				No incident selected.
			</div>
		)
	}

	if (loading && !dialog) {
		return (
			<p className="py-4 text-center text-sm text-muted-foreground">
				Loading incident...
			</p>
		)
	}

	if (error || !dialog) {
		return (
			<div className="space-y-4">
				<Button variant="ghost" size="sm" onClick={handleBack}>
					<ArrowLeft className="mr-2 h-4 w-4" />
					Back to list
				</Button>
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{error ?? 'Incident not found'}
				</div>
			</div>
		)
	}

	const markdownSection = (
		field: PostmortemScalarField,
		title: string,
		emptyText: string,
		defaultExpanded = true,
	) =>
		postmortem && (
			<EditableMarkdownSection
				title={title}
				content={postmortem[field]}
				emptyText={emptyText}
				locked={postmortem.locks.includes(field)}
				onSave={saveScalar(field)}
				onUnlock={unlockScalar(field)}
				defaultExpanded={defaultExpanded}
				onDialogLink={setActiveDialogId}
			/>
		)

	const actionsCard = (list: 'shortTerm' | 'longTerm', title: string, description: string) =>
		postmortem && (
			<EditableListCard
				title={title}
				description={description}
				items={postmortem[list]}
				fields={ACTION_FIELDS}
				toFields={(action) => ({ text: action.text, status: action.status })}
				renderItem={(action) => <ActionText action={action} onOpenPlan={handleOpenPlan} />}
				canDelete={(action) => action.status !== 'in_plan'}
				leading={(action) => (
					<Checkbox
						className="mt-0.5"
						checked={selected.has(action.id)}
						disabled={!isActionSelectable(action)}
						onCheckedChange={(checked) => handleToggleAction(action.id, checked === true)}
						aria-label={`Select "${action.text}" for a plan`}
					/>
				)}
				emptyText="No actions yet."
				addLabel="Add action"
				onAdd={addItem(list)}
				onUpdate={updateItem(list)}
				onDelete={deleteItem(list)}
			/>
		)

	return (
		<div className="space-y-6">
			<div className="flex items-center justify-between">
				<Button variant="ghost" size="sm" onClick={handleBack}>
					<ArrowLeft className="mr-2 h-4 w-4" />
					Back to list
				</Button>
				<div className="flex items-center gap-2">
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={!postmortem || closed || runActive}
						onClick={() => setCloseOpen(true)}
					>
						<CheckCheck className="mr-2 h-4 w-4" />
						{closed ? 'Closed' : 'Close incident'}
					</Button>
					<IconButton
						type="button"
						variant="outline"
						size="sm"
						className="h-8 w-8"
						disabled={!postmortem}
						onClick={handleDownload}
						tooltip="Download post-mortem as Markdown"
					>
						<FileText className="h-4 w-4" />
					</IconButton>
				</div>
			</div>

			<div className="text-center">
				<h2
					className={cn(
						'text-2xl font-bold tracking-tight',
						!dialog.title && 'font-mono text-lg',
					)}
					title={dialog.title || dialog.id}
				>
					{dialog.title || dialog.id}
				</h2>
			</div>

			{postmortemError && (
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{postmortemError}
				</div>
			)}

			{!postmortem ? (
				!postmortemError && (
					<p className="py-4 text-center text-sm text-muted-foreground">
						Loading post-mortem...
					</p>
				)
			) : (
				<>
					<PostmortemHeader postmortem={postmortem} onEdit={edit} />

					{run && (
						<InvestigationCard
							run={run}
							onStop={handleStopRun}
							onOpenTranscript={setActiveDialogId}
						/>
					)}

					{markdownSection(
						'summary',
						'Summary',
						'No summary yet. Describe the problem in the chat, or paste the alert or ticket, and the investigation fills this in.',
					)}
					{markdownSection('hypothesis', 'Working hypothesis', 'No hypothesis yet.')}

					<EditableListCard
						title="Root cause"
						items={postmortem.rootCauses}
						fields={ROOT_CAUSE_FIELDS}
						toFields={(cause) => ({ text: cause.text })}
						renderItem={(cause) => <span className="whitespace-pre-wrap">{cause.text}</span>}
						emptyText="Not established yet."
						addLabel="Add root cause"
						onAdd={addItem('rootCauses')}
						onUpdate={updateItem('rootCauses')}
						onDelete={deleteItem('rootCauses')}
					/>

					<EditableListCard
						title="Impacted areas"
						items={postmortem.impacted}
						fields={IMPACTED_FIELDS}
						toFields={(area) => ({
							name: area.name,
							environment: area.environment,
							severity: area.severity,
							description: area.description,
						})}
						renderItem={(area) => (
							<div>
								<span className="font-medium">{area.name}</span>
								{area.environment && (
									<span className="text-muted-foreground"> · {area.environment}</span>
								)}
								{area.severity && (
									<span className="text-muted-foreground"> · {area.severity}</span>
								)}
								{area.description && (
									<p className="whitespace-pre-wrap text-muted-foreground">
										{area.description}
									</p>
								)}
							</div>
						)}
						emptyText="None identified yet."
						addLabel="Add impacted area"
						onAdd={addItem('impacted')}
						onUpdate={updateItem('impacted')}
						onDelete={deleteItem('impacted')}
					/>

					<EditableListCard
						title="Timeline"
						description="Times are shown in your local time zone and stored in UTC."
						items={postmortem.timeline}
						fields={TIMELINE_FIELDS}
						toFields={(event) => ({ at: event.at, event: event.event, source: event.source })}
						renderItem={(event) => (
							<div className="grid grid-cols-1 gap-x-4 sm:grid-cols-[11rem_1fr]">
								<span className="font-mono text-xs text-muted-foreground sm:pt-0.5">
									{event.at ? formatIncidentTime(event.at) : 'time unknown'}
								</span>
								<span className="whitespace-pre-wrap">
									{event.event}
									{event.source && (
										<span className="text-muted-foreground"> — {event.source}</span>
									)}
								</span>
							</div>
						)}
						emptyText="No events yet."
						addLabel="Add event"
						onAdd={addItem('timeline')}
						onUpdate={updateItem('timeline')}
						onDelete={deleteItem('timeline')}
					/>

					<FindingsCard findings={postmortem.findings} onOpenTranscript={setActiveDialogId} />

					{actionsCard('shortTerm', 'Short-term changes', 'What to do here and now to mitigate.')}
					{actionsCard('longTerm', 'Long-term fixes', 'What prevents a recurrence.')}

					<Card>
						<CardContent className="flex flex-wrap items-center gap-3 pt-6">
							<p className="text-sm text-muted-foreground">
								{selected.size === 0
									? 'Tick corrective actions above to hand them to the change-management track.'
									: `${selected.size} action${selected.size === 1 ? '' : 's'} selected.`}
							</p>
							<Button
								type="button"
								className="ml-auto"
								disabled={selected.size === 0 || creatingPlan}
								onClick={() => void handleCreatePlan()}
							>
								<GitPullRequestArrow className="mr-2 h-4 w-4" />
								{creatingPlan ? 'Creating plan...' : 'Create plan'}
							</Button>
							{planError && <p className="w-full text-sm text-destructive">{planError}</p>}
						</CardContent>
					</Card>

					{markdownSection('impact', 'Impact', 'No impact assessment yet.', false)}
					{markdownSection('closure', 'Closure', 'Written when the incident is closed.', closed)}

					<CloseIncidentDialog
						open={closeOpen}
						onOpenChange={setCloseOpen}
						resolvedAt={postmortem.resolvedAt}
						onConfirm={handleClose}
					/>
				</>
			)}
		</div>
	)
}
