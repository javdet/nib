import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type KeyboardEvent,
} from 'react'
import { useNavigate, useParams } from 'react-router'
import {
	ArrowDown,
	ArrowLeft,
	ChevronDown,
	ChevronRight,
	Download,
	Pencil,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { IconButton } from '@/components/ui/icon-button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { MarkdownMessage } from '@/components/markdown-message'
import { cn } from '@/lib/utils'
import { downloadTextFile } from '@/lib/download'
import {
	getActionPlanExecRuns,
	type ActionExecRuns,
	getDialog,
	getDialogActionPlan,
	getDialogDag,
	getDialogSummary,
	getPlanState,
	getPlanFanout,
	startPlanFanout,
	cancelPlanFanout,
	stopExecution,
	listDialogChildren,
	openDialogActivity,
	updateActionPlan,
	updateActionPlanChecks,
	updateActionPlanComments,
	reorderActionPlanItems,
	updatePlanSchedule,
	updateDialogSummary,
	updateDialogTitle,
	updateDialogCategories,
	type ActionPlan,
	type ActionStep,
	type ActionPlanScope,
	type Dialog as DialogItem,
	type FanoutRun,
	type PlanStatus,
} from '@/features/dialogs/api/dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { dialogDisplayTitle } from '@/features/dialogs/lib/dialog-title'
import { useMode } from '@/features/modes/mode-context'
import {
	getExecutorConfig,
	type ExecutorType,
} from '@/features/executor/api/executor'
import { ActionPlanView } from '../components/action-plan-view'
import { DagView } from '../components/dag-view'
import { PlanMetaTable } from '../components/plan-meta-table'
import { PlanProgressBar } from '../components/plan-progress-bar'
import {
	buildPlanMarkdown,
	planMarkdownFileName,
} from '../lib/plan-markdown'
import { actionPlanNumberForKey } from '../lib/action-plan-number'
import {
	executeActionMessage,
	restartActionMessage,
} from '../lib/execute-action-message'
import {
	planStageMessage,
	processPlanMessage,
	replanAllStagesMessage,
} from '../lib/plan-message'

// commandActionTypes are the step types executed by copying commands into a
// terminal, so their command field is offered for editing.
const commandActionTypes = ['shell', 'curl']

function getActionStep(plan: ActionPlan, key: string): ActionStep | null {
	const rollbackMatch = /^rollback\.(\d+)$/.exec(key)
	if (rollbackMatch) {
		const idx = Number(rollbackMatch[1])
		return plan.rollback[idx] ?? null
	}

	const stepMatch = /^s(\d+)\.step(\d+)$/.exec(key)
	if (stepMatch) {
		const stageIdx = Number(stepMatch[1])
		const stepIdx = Number(stepMatch[2])
		return plan.stages[stageIdx]?.steps[stepIdx] ?? null
	}

	return null
}

function hasCommandField(step: ActionStep): boolean {
	return (
		commandActionTypes.includes(step.type?.trim().toLowerCase()) ||
		!!step.command?.trim()
	)
}

function setActionFields(
	plan: ActionPlan,
	key: string,
	fields: Pick<ActionStep, 'action' | 'command'>,
): ActionPlan | null {
	const next: ActionPlan = {
		stages: plan.stages.map((stage) => ({
			...stage,
			steps: stage.steps.map((step) => ({ ...step })),
			checks: stage.checks.map((check) => ({ ...check })),
		})),
		rollback: plan.rollback.map((step) => ({ ...step })),
	}

	const rollbackMatch = /^rollback\.(\d+)$/.exec(key)
	if (rollbackMatch) {
		const idx = Number(rollbackMatch[1])
		if (!next.rollback[idx]) return null
		next.rollback[idx] = { ...next.rollback[idx], ...fields }
		return next
	}

	const stepMatch = /^s(\d+)\.step(\d+)$/.exec(key)
	if (stepMatch) {
		const stageIdx = Number(stepMatch[1])
		const stepIdx = Number(stepMatch[2])
		const stage = next.stages[stageIdx]
		const step = stage?.steps[stepIdx]
		if (!stage || !step) return null
		stage.steps[stepIdx] = { ...step, ...fields }
		return next
	}

	return null
}

export function WorkplaceDetail() {
	const { id } = useParams<{ id: string }>()
	const navigate = useNavigate()
	const { setActiveDialogId, bumpDialogsVersion, dialogsVersion, actionPlanVersion, enqueuePendingMessage } =
		useDialog()
	const { selectMode } = useMode()
	const [dialog, setDialog] = useState<DialogItem | null>(null)
	const [summaryContent, setSummaryContent] = useState<string | null>(null)
	const [dagContent, setDagContent] = useState<string | null>(null)
	const [fanoutRun, setFanoutRun] = useState<FanoutRun | null>(null)
	// The plan's decompose transcript, when it has one. Only an orchestrator plan
	// does; a legacy decompose root is its own.
	const [decomposeDialogId, setDecomposeDialogId] = useState<string | null>(null)
	const [actionPlan, setActionPlan] = useState<ActionPlan | null>(null)
	const [actionPlanChecked, setActionPlanChecked] = useState<string[]>([])
	const [actionPlanComments, setActionPlanComments] = useState<
		Record<string, string>
	>({})
	const [planStatus, setPlanStatus] = useState<PlanStatus>('draft')
	const [planScheduledAt, setPlanScheduledAt] = useState(0)
	const [savingSchedule, setSavingSchedule] = useState(false)
	const [commentDialogKey, setCommentDialogKey] = useState<string | null>(null)
	const [commentDraft, setCommentDraft] = useState('')
	const [savingComment, setSavingComment] = useState(false)
	const [editActionKey, setEditActionKey] = useState<string | null>(null)
	const [editActionDraft, setEditActionDraft] = useState('')
	const [editCommandDraft, setEditCommandDraft] = useState('')
	const [savingAction, setSavingAction] = useState(false)
	const [reordering, setReordering] = useState(false)
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)
	const [summaryExpanded, setSummaryExpanded] = useState(true)
	const [dagExpanded, setDagExpanded] = useState(true)
	const [actionListExpanded, setActionListExpanded] = useState(true)
	const [stagesExpanded, setStagesExpanded] = useState(true)
	const [processingPlan, setProcessingPlan] = useState(false)
	const [isEditingTitle, setIsEditingTitle] = useState(false)
	const [titleDraft, setTitleDraft] = useState('')
	const [savingTitle, setSavingTitle] = useState(false)
	const [isEditingSummary, setIsEditingSummary] = useState(false)
	const [summaryDraft, setSummaryDraft] = useState('')
	const [savingSummary, setSavingSummary] = useState(false)
	const [savingCategories, setSavingCategories] = useState(false)
	const [liveActionPlanVersion, setLiveActionPlanVersion] = useState(0)
	const [execRuns, setExecRuns] = useState<ActionExecRuns>({})
	const [executorType, setExecutorType] = useState<ExecutorType | null>(null)
	const [simplifiedView, setSimplifiedView] = useState(
		() => localStorage.getItem('plan-simplified-view') !== 'false',
	)
	const titleInputRef = useRef<HTMLInputElement>(null)
	const summaryTextareaRef = useRef<HTMLTextAreaElement>(null)
	const skipTitleSaveRef = useRef(false)

	const loadDetail = useCallback(async (dialogId: string) => {
		setLoading(true)
		setError(null)
		try {
			const [
				dialogData,
				summary,
				dag,
				planState,
				actionPlanData,
				run,
				runs,
			] = await Promise.all([
				getDialog(dialogId),
				getDialogSummary(dialogId),
				getDialogDag(dialogId),
				getPlanState(dialogId),
				getDialogActionPlan(dialogId),
				getPlanFanout(dialogId),
				getActionPlanExecRuns(dialogId),
			])
			setDialog(dialogData)
			setSummaryContent(summary)
			setDagContent(dag)
			setFanoutRun(run)
			setActionPlan(actionPlanData?.plan ?? null)
			setActionPlanChecked(actionPlanData?.checked ?? [])
			setActionPlanComments(actionPlanData?.comments ?? {})
			setExecRuns(runs)
			setPlanStatus(planState.status)
			setPlanScheduledAt(planState.scheduledAt)
		} catch (err) {
			setDialog(null)
			setSummaryContent(null)
			setDagContent(null)
			setFanoutRun(null)
			setActionPlan(null)
			setActionPlanChecked([])
			setActionPlanComments({})
			setExecRuns({})
			setPlanStatus('draft')
			setPlanScheduledAt(0)
			setError(
				err instanceof Error ? err.message : 'Failed to load plan',
			)
		} finally {
			setLoading(false)
		}
	}, [])

	useEffect(() => {
		if (!id) return
		setActiveDialogId(id)
	}, [id, setActiveDialogId])

	// The executor type decides whether code actions can run at all; a failed
	// read leaves it null so the Execute button keeps its default behaviour.
	useEffect(() => {
		let cancelled = false
		void getExecutorConfig()
			.then((cfg) => {
				if (!cancelled) setExecutorType(cfg.type)
			})
			.catch(() => {})

		return () => {
			cancelled = true
		}
	}, [])

	useEffect(() => {
		if (!id) return
		void loadDetail(id)
	}, [id, loadDetail, dialogsVersion])

	useEffect(() => {
		if (!id) return

		// Stages land one at a time, so the plan is re-read on every stage event
		// rather than only when the whole run finishes.
		const close = openDialogActivity(id, (ev) => {
			if (ev.kind === 'action_plan_updated') {
				setLiveActionPlanVersion((v) => v + 1)
				return
			}
			if (
				ev.kind === 'plan_stage_started' ||
				ev.kind === 'plan_stage_done' ||
				ev.kind === 'plan_stage_failed' ||
				ev.kind === 'plan_fanout_done'
			) {
				void getPlanFanout(id).then(setFanoutRun).catch(() => {})
				return
			}
			if (
				ev.kind === 'action_exec_started' ||
				ev.kind === 'action_exec_done' ||
				ev.kind === 'action_exec_failed'
			) {
				void getActionPlanExecRuns(id).then(setExecRuns).catch(() => {})
			}
		})

		return close
	}, [id])

	useEffect(() => {
		if (!id || (actionPlanVersion === 0 && liveActionPlanVersion === 0)) {
			return
		}

		let cancelled = false
		void getDialogActionPlan(id)
			.then((actionPlanData) => {
				if (cancelled) return
				setActionPlan(actionPlanData?.plan ?? null)
				setActionPlanChecked(actionPlanData?.checked ?? [])
				setActionPlanComments(actionPlanData?.comments ?? {})
			})
			.catch((err) => {
				if (cancelled) return
				setError(
					err instanceof Error
						? err.message
						: 'Failed to refresh action plan',
				)
			})

		return () => {
			cancelled = true
		}
	}, [id, actionPlanVersion, liveActionPlanVersion])

	useEffect(() => {
		if (!isEditingTitle) return
		titleInputRef.current?.focus()
		titleInputRef.current?.select()
	}, [isEditingTitle])

	useEffect(() => {
		if (!isEditingSummary) return
		summaryTextareaRef.current?.focus()
		summaryTextareaRef.current?.select()
	}, [isEditingSummary])

	const handleStartTitleEdit = useCallback(() => {
		if (!dialog) return
		skipTitleSaveRef.current = false
		setTitleDraft(dialogDisplayTitle(dialog))
		setIsEditingTitle(true)
	}, [dialog])

	const handleCancelTitleEdit = useCallback(() => {
		skipTitleSaveRef.current = true
		setIsEditingTitle(false)
		setTitleDraft('')
	}, [])

	const handleSaveTitle = useCallback(async () => {
		if (!id || !dialog || savingTitle) return

		const nextTitle = titleDraft.trim()
		if (!nextTitle) {
			setError('Plan name cannot be empty')
			return
		}

		const currentTitle = dialogDisplayTitle(dialog)
		if (nextTitle === currentTitle) {
			setIsEditingTitle(false)
			setTitleDraft('')
			return
		}

		setSavingTitle(true)
		setError(null)
		try {
			const updated = await updateDialogTitle(id, nextTitle)
			setDialog(updated)
			setIsEditingTitle(false)
			setTitleDraft('')
			bumpDialogsVersion()
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to rename plan',
			)
		} finally {
			setSavingTitle(false)
		}
	}, [id, dialog, titleDraft, savingTitle, bumpDialogsVersion])

	const handleStartSummaryEdit = useCallback(() => {
		setSummaryDraft(summaryContent ?? '')
		setIsEditingSummary(true)
	}, [summaryContent])

	const handleCancelSummaryEdit = useCallback(() => {
		setIsEditingSummary(false)
		setSummaryDraft('')
	}, [])

	const handleSaveSummary = useCallback(async () => {
		if (!id || savingSummary) return

		const nextSummary = summaryDraft.trim()
		if (!nextSummary) {
			setError('Summary cannot be empty')
			return
		}

		const currentSummary = summaryContent?.trim() ?? ''
		if (nextSummary === currentSummary) {
			setIsEditingSummary(false)
			setSummaryDraft('')
			return
		}

		setSavingSummary(true)
		setError(null)
		try {
			const updated = await updateDialogSummary(id, nextSummary)
			setSummaryContent(updated)
			setIsEditingSummary(false)
			setSummaryDraft('')
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to update summary',
			)
		} finally {
			setSavingSummary(false)
		}
	}, [id, summaryDraft, summaryContent, savingSummary])

	const handleSaveCategories = useCallback(
		async (nextCategories: string[]) => {
			if (!id || !dialog || savingCategories) return

			const previous = dialog.categories ?? []
			const unchanged =
				previous.length === nextCategories.length &&
				previous.every(
					(category, index) => category === nextCategories[index],
				)
			if (unchanged) {
				return
			}

			setDialog((current) =>
				current ? { ...current, categories: nextCategories } : current,
			)
			setSavingCategories(true)
			setError(null)
			try {
				const updated = await updateDialogCategories(id, nextCategories)
				setDialog(updated)
			} catch (err) {
				setDialog((current) =>
					current ? { ...current, categories: previous } : current,
				)
				setError(
					err instanceof Error
						? err.message
						: 'Failed to update tool categories',
				)
			} finally {
				setSavingCategories(false)
			}
		},
		[id, dialog, savingCategories],
	)

	const handleBack = useCallback(() => {
		void navigate('/workplace')
	}, [navigate])

	const handleSimplifiedViewChange = useCallback((next: boolean) => {
		setSimplifiedView(next)
		localStorage.setItem('plan-simplified-view', String(next))
	}, [])

	const handleDownloadPlan = useCallback(() => {
		if (!dialog) return

		const title = dialogDisplayTitle(dialog)
		const markdown = buildPlanMarkdown({
			title,
			planStatus,
			scheduledAt: planScheduledAt,
			createdAt: dialog.createdAt,
			updatedAt: dialog.updatedAt,
			summary: summaryContent,
			dag: dagContent,
			plan: actionPlan,
			checked: actionPlanChecked,
			comments: actionPlanComments,
		})

		downloadTextFile(planMarkdownFileName(title), markdown)
	}, [
		dialog,
		planStatus,
		planScheduledAt,
		summaryContent,
		dagContent,
		actionPlan,
		actionPlanChecked,
		actionPlanComments,
	])

	const handleActionPlanToggle = useCallback(
		(key: string, nextChecked: boolean) => {
			if (!id) return

			const prevChecked = actionPlanChecked
			const prevStatus = planStatus
			const next = nextChecked
				? [...actionPlanChecked, key]
				: actionPlanChecked.filter((item) => item !== key)

			setActionPlanChecked(next)

			// Status is derived from full plan progress on the server; taking it
			// from the response keeps a single source of truth.
			void updateActionPlanChecks(id, next)
				.then((res) => setPlanStatus(res.planStatus))
				.catch((err) => {
					setActionPlanChecked(prevChecked)
					setPlanStatus(prevStatus)
					setError(
						err instanceof Error
							? err.message
							: 'Failed to update action plan checks',
					)
				})
		},
		[id, actionPlanChecked, planStatus],
	)

	const handleOpenComment = useCallback(
		(key: string) => {
			setCommentDialogKey(key)
			setCommentDraft(actionPlanComments[key] ?? '')
		},
		[actionPlanComments],
	)

	const handleCloseComment = useCallback(() => {
		setCommentDialogKey(null)
		setCommentDraft('')
	}, [])

	const handleSaveComment = useCallback(async () => {
		if (!id || commentDialogKey === null || savingComment) return

		const trimmed = commentDraft.trim()
		const next = { ...actionPlanComments }
		if (trimmed) {
			next[commentDialogKey] = trimmed
		} else {
			delete next[commentDialogKey]
		}

		setSavingComment(true)
		setError(null)
		try {
			const saved = await updateActionPlanComments(id, next)
			setActionPlanComments(saved)
			handleCloseComment()
		} catch (err) {
			setError(
				err instanceof Error
					? err.message
					: 'Failed to save action comment',
			)
		} finally {
			setSavingComment(false)
		}
	}, [
		id,
		commentDialogKey,
		commentDraft,
		actionPlanComments,
		savingComment,
		handleCloseComment,
	])

	const handleOpenEditAction = useCallback(
		(key: string) => {
			if (!actionPlan) return
			const step = getActionStep(actionPlan, key)
			if (!step) return
			setEditActionKey(key)
			setEditActionDraft(step.action)
			setEditCommandDraft(step.command ?? '')
		},
		[actionPlan],
	)

	const handleCloseEditAction = useCallback(() => {
		setEditActionKey(null)
		setEditActionDraft('')
		setEditCommandDraft('')
	}, [])

	const handleSaveAction = useCallback(async () => {
		if (!id || !actionPlan || editActionKey === null || savingAction) return

		const trimmed = editActionDraft.trim()
		if (!trimmed) {
			setError('Action description cannot be empty')
			return
		}

		const current = getActionStep(actionPlan, editActionKey)
		if (!current) {
			setError('Action not found')
			return
		}

		// An empty command drops the field rather than storing a blank string,
		// so a step never claims to carry commands it does not have.
		const command = hasCommandField(current)
			? editCommandDraft.trim() || undefined
			: current.command
		if (trimmed === current.action && command === current.command) {
			handleCloseEditAction()
			return
		}

		const nextPlan = setActionFields(actionPlan, editActionKey, {
			action: trimmed,
			command,
		})
		if (!nextPlan) {
			setError('Action not found')
			return
		}

		setSavingAction(true)
		setError(null)
		try {
			const saved = await updateActionPlan(id, nextPlan)
			setActionPlan(saved.plan)
			setPlanStatus(saved.planStatus)
			handleCloseEditAction()
		} catch (err) {
			setError(
				err instanceof Error
					? err.message
					: 'Failed to save action description',
			)
		} finally {
			setSavingAction(false)
		}
	}, [
		id,
		actionPlan,
		editActionKey,
		editActionDraft,
		editCommandDraft,
		savingAction,
		handleCloseEditAction,
	])

	const handleEditActionKeyDown = useCallback(
		(e: KeyboardEvent<HTMLTextAreaElement>) => {
			if (e.key === 'Escape') {
				e.preventDefault()
				handleCloseEditAction()
			}
			if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
				e.preventDefault()
				void handleSaveAction()
			}
		},
		[handleCloseEditAction, handleSaveAction],
	)

	const editingStep =
		actionPlan && editActionKey !== null
			? getActionStep(actionPlan, editActionKey)
			: null
	const editingStepHasCommand = !!editingStep && hasCommandField(editingStep)

	const handleReorder = useCallback(
		async (
			scope: ActionPlanScope,
			stage: number,
			from: number,
			to: number,
		) => {
			if (!id || reordering) return

			setReordering(true)
			setError(null)
			try {
				const result = await reorderActionPlanItems(
					id,
					scope,
					stage,
					from,
					to,
				)
				setActionPlan(result.plan)
				setActionPlanChecked(result.checked)
				setActionPlanComments(result.comments)
				if (result.planStatus) {
					setPlanStatus(result.planStatus)
				}
			} catch (err) {
				setError(
					err instanceof Error
						? err.message
						: 'Failed to reorder action plan items',
				)
			} finally {
				setReordering(false)
			}
		},
		[id, reordering],
	)

	// Execution is driven from the planning chat: the button sends the sentence
	// an operator could have typed, and the agent turns it into an execute_action
	// call. Routing both through the same path is what makes a hand-typed request
	// -- in any wording, in any language -- behave exactly like the button.
	const sendActionCommand = useCallback(
		(key: string, compose: (number: string) => string) => {
			if (!id) return

			const number = actionPlanNumberForKey(key)
			if (!number) {
				setError('This action has no number yet; reload the plan and try again.')
				return
			}

			enqueuePendingMessage({ dialogId: id, text: compose(number) })
			setActiveDialogId(id)
		},
		[id, enqueuePendingMessage, setActiveDialogId],
	)

	const handleExecuteAction = useCallback(
		(_step: ActionStep, key: string) => {
			sendActionCommand(key, executeActionMessage)
		},
		[sendActionCommand],
	)

	const handleRestartAction = useCallback(
		(key: string) => {
			sendActionCommand(key, restartActionMessage)
		},
		[sendActionCommand],
	)

	const handleScheduleChange = useCallback(
		async (nextUnix: number) => {
			if (!id || savingSchedule) return

			const prevScheduledAt = planScheduledAt
			const prevStatus = planStatus
			setPlanScheduledAt(nextUnix)
			if (nextUnix > 0 && planStatus === 'draft') {
				setPlanStatus('scheduled')
			} else if (nextUnix <= 0 && planStatus === 'scheduled') {
				setPlanStatus('draft')
			}

			setSavingSchedule(true)
			setError(null)
			try {
				const result = await updatePlanSchedule(id, nextUnix)
				setPlanScheduledAt(result.scheduledAt)
				setPlanStatus(result.status)
			} catch (err) {
				setPlanScheduledAt(prevScheduledAt)
				setPlanStatus(prevStatus)
				setError(
					err instanceof Error
						? err.message
						: 'Failed to update plan schedule',
				)
			} finally {
				setSavingSchedule(false)
			}
		},
		[id, savingSchedule, planScheduledAt, planStatus],
	)

	// The decompose sub-agent's transcript is where the research behind the
	// summary and the DAG lives. Nothing else links to it, so the Summary card
	// does.
	useEffect(() => {
		if (!id || dialog?.mode !== 'main') {
			setDecomposeDialogId(null)
			return
		}

		let cancelled = false
		void listDialogChildren(id)
			.then((children) => {
				if (cancelled) return
				const child = children.find((c) => c.mode === 'decompose')
				setDecomposeDialogId(child?.id ?? null)
			})
			.catch(() => {
				// A missing transcript costs a link, not the page.
				if (!cancelled) setDecomposeDialogId(null)
			})

		return () => {
			cancelled = true
		}
	}, [id, dialog?.mode, summaryContent])

	const handleOpenDecomposeDialog = useCallback(() => {
		if (!decomposeDialogId) return
		selectMode('decompose')
		setActiveDialogId(decomposeDialogId)
	}, [decomposeDialogId, selectMode, setActiveDialogId])

	const handleOpenStageDialog = useCallback(
		(stageDialogId: string) => {
			selectMode('plan')
			setActiveDialogId(stageDialogId)
		},
		[selectMode, setActiveDialogId],
	)

	// Planning is driven from the main chat, the same way execution is: the
	// button sends the sentence an operator could have typed and the orchestrator
	// launches the plan sub-agent, which fans the DAG out to one agent per stage.
	//
	// A plan whose root is a legacy decompose dialog has no orchestrator to send
	// it to, so it keeps the endpoint that used to drive this button.
	const handleProcessPlan = useCallback(async () => {
		if (!id) return

		if (dialog?.mode === 'decompose') {
			setProcessingPlan(true)
			setError(null)
			try {
				setFanoutRun(await startPlanFanout(id))
				bumpDialogsVersion()
			} catch (err) {
				setError(
					err instanceof Error ? err.message : 'Failed to start planning',
				)
			} finally {
				setProcessingPlan(false)
			}
			return
		}

		enqueuePendingMessage({
			dialogId: id,
			text: actionPlan ? replanAllStagesMessage() : processPlanMessage(),
		})
		setActiveDialogId(id)
	}, [
		id,
		dialog?.mode,
		actionPlan,
		enqueuePendingMessage,
		setActiveDialogId,
		bumpDialogsVersion,
	])

	// Replanning one stage is the "let's work on stage 2" request, sent as the
	// sentence rather than as a targeted endpoint call for the same reason.
	const handleReplanStage = useCallback(
		(stageTitle: string) => {
			if (!id) return
			enqueuePendingMessage({ dialogId: id, text: planStageMessage(stageTitle) })
			setActiveDialogId(id)
		},
		[id, enqueuePendingMessage, setActiveDialogId],
	)

	// A force stop is an endpoint rather than a chat message: it has to work
	// while the agent loop holding the execution is wedged, which is exactly when
	// it is reached for.
	const handleStopExecution = useCallback(async () => {
		if (!id) return

		setError(null)
		try {
			await stopExecution()
			setExecRuns(await getActionPlanExecRuns(id))
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to stop the execution',
			)
		}
	}, [id])

	const handleCancelFanout = useCallback(async () => {
		if (!id) return
		try {
			await cancelPlanFanout(id)
			setFanoutRun(await getPlanFanout(id))
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to stop planning',
			)
		}
	}, [id])

	if (!id) {
		return (
			<div className="text-sm text-muted-foreground">
				No plan selected.
			</div>
		)
	}

	if (loading && !dialog) {
		return (
			<p className="py-4 text-center text-sm text-muted-foreground">
				Loading plan...
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
					{error ?? 'Plan not found'}
				</div>
			</div>
		)
	}

	const canProcessPlan = Boolean(dagContent)
	const fanoutRunning = fanoutRun?.status === 'running'
	const isDecomposed = Boolean(summaryContent && dagContent)
	const processPlanDisabled =
		processingPlan || fanoutRunning || !canProcessPlan
	const showProcessPlanHero =
		!actionPlan && !processingPlan && !fanoutRunning

	return (
		<div className="space-y-6">
			<div className="flex items-center justify-between">
				<Button variant="ghost" size="sm" onClick={handleBack}>
					<ArrowLeft className="mr-2 h-4 w-4" />
					Back to list
				</Button>
				<div className="flex items-center gap-2">
					<div
						className="inline-flex rounded-md border p-1"
						role="group"
						aria-label="Plan view"
					>
						<Button
							type="button"
							size="sm"
							variant={simplifiedView ? 'secondary' : 'ghost'}
							className="rounded-sm"
							onClick={() => handleSimplifiedViewChange(true)}
							aria-pressed={simplifiedView}
						>
							Simplified
						</Button>
						<Button
							type="button"
							size="sm"
							variant={!simplifiedView ? 'secondary' : 'ghost'}
							className="rounded-sm"
							onClick={() => handleSimplifiedViewChange(false)}
							aria-pressed={!simplifiedView}
						>
							Detailed
						</Button>
					</div>
					<IconButton
						type="button"
						variant="outline"
						size="sm"
						className="h-8 w-8"
						onClick={handleDownloadPlan}
						tooltip="Download plan as Markdown"
					>
						<Download className="h-4 w-4" />
					</IconButton>
				</div>
			</div>

			<div className="text-center">
				{isEditingTitle ? (
					<div className="mx-auto flex max-w-xl items-center justify-center gap-2">
						<Input
							ref={titleInputRef}
							value={titleDraft}
							onChange={(e) => setTitleDraft(e.target.value)}
							onKeyDown={(e) => {
								if (e.key === 'Enter') {
									e.preventDefault()
									void handleSaveTitle()
								}
								if (e.key === 'Escape') {
									e.preventDefault()
									handleCancelTitleEdit()
								}
							}}
							onBlur={() => {
								if (skipTitleSaveRef.current) {
									skipTitleSaveRef.current = false
									return
								}
								void handleSaveTitle()
							}}
							disabled={savingTitle}
							className="text-center text-2xl font-bold tracking-tight"
							aria-label="Plan name"
						/>
					</div>
				) : (
					<div className="flex items-center justify-center gap-2">
						<h2
							className="text-2xl font-bold tracking-tight"
							title={dialogDisplayTitle(dialog)}
						>
							{dialogDisplayTitle(dialog)}
						</h2>
						<IconButton
							type="button"
							variant="ghost"
							className="h-8 w-8 shrink-0 text-muted-foreground"
							onMouseDown={(e) => e.preventDefault()}
							onClick={handleStartTitleEdit}
							tooltip="Rename plan"
						>
							<Pencil className="h-4 w-4" />
						</IconButton>
					</div>
				)}
			</div>

			<PlanProgressBar plan={actionPlan} checked={actionPlanChecked} />

			{!simplifiedView && (
				<PlanMetaTable
					planStatus={planStatus}
					scheduledAt={planScheduledAt}
					onScheduleChange={(nextUnix) => void handleScheduleChange(nextUnix)}
					scheduleDisabled={!isDecomposed || savingSchedule}
					showSchedulingHint={!isDecomposed}
					categories={dialog.categories ?? []}
					onCategoriesChange={(next) => void handleSaveCategories(next)}
					disabled={savingCategories}
				/>
			)}

			<Card>
				<CardHeader>
					<div
						role="button"
						tabIndex={0}
						aria-expanded={summaryExpanded}
						className="flex cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						onClick={() => setSummaryExpanded((prev) => !prev)}
						onKeyDown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault()
								setSummaryExpanded((prev) => !prev)
							}
						}}
					>
						{summaryExpanded ? (
							<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
						) : (
							<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
						)}
						<CardTitle>Summary</CardTitle>
						{decomposeDialogId && !isEditingSummary && (
							<Button
								type="button"
								variant="ghost"
								size="sm"
								className="ml-auto shrink-0 text-muted-foreground"
								onMouseDown={(e) => e.preventDefault()}
								onClick={(e) => {
									e.stopPropagation()
									handleOpenDecomposeDialog()
								}}
							>
								Decomposition chat
							</Button>
						)}
						{!isEditingSummary && (
							<IconButton
								type="button"
								variant="ghost"
								className={cn(
									'h-8 w-8 shrink-0 text-muted-foreground',
									!decomposeDialogId && 'ml-auto',
								)}
								onMouseDown={(e) => e.preventDefault()}
								onClick={(e) => {
									e.stopPropagation()
									handleStartSummaryEdit()
								}}
								tooltip="Edit summary"
							>
								<Pencil className="h-4 w-4" />
							</IconButton>
						)}
					</div>
				</CardHeader>
				{summaryExpanded && (
					<CardContent>
						{isEditingSummary ? (
							<div className="space-y-3">
								<Textarea
									ref={summaryTextareaRef}
									value={summaryDraft}
									onChange={(e) => setSummaryDraft(e.target.value)}
									onKeyDown={(e) => {
										if (e.key === 'Escape') {
											e.preventDefault()
											handleCancelSummaryEdit()
										}
										if (
											e.key === 'Enter' &&
											(e.metaKey || e.ctrlKey)
										) {
											e.preventDefault()
											void handleSaveSummary()
										}
									}}
									disabled={savingSummary}
									className="min-h-[120px] whitespace-pre-wrap"
									aria-label="Summary"
								/>
								<div className="flex gap-2">
									<Button
										type="button"
										size="sm"
										onClick={() => void handleSaveSummary()}
										disabled={savingSummary}
									>
										{savingSummary ? 'Saving...' : 'Save'}
									</Button>
									<Button
										type="button"
										variant="outline"
										size="sm"
										onClick={handleCancelSummaryEdit}
										disabled={savingSummary}
									>
										Cancel
									</Button>
								</div>
							</div>
						) : summaryContent ? (
							<div className="text-sm">
								<MarkdownMessage content={summaryContent} />
							</div>
						) : (
							<p className="text-sm text-muted-foreground">
								No summary yet for this conversation.
							</p>
						)}
					</CardContent>
				)}
			</Card>

			{/* The glass sheet is the whole section, not a panel inside a card:
			    the DAG is drawn on it the way it would be on a board. */}
			<div className="dag-board">
				<CardHeader>
					<div
						role="button"
						tabIndex={0}
						aria-expanded={dagExpanded}
						className="flex cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						onClick={() => setDagExpanded((prev) => !prev)}
						onKeyDown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault()
								setDagExpanded((prev) => !prev)
							}
						}}
					>
						{dagExpanded ? (
							<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
						) : (
							<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
						)}
						<CardTitle>DAG</CardTitle>
					</div>
				</CardHeader>
				{dagExpanded && (
					<CardContent>
						{dagContent ? (
							<DagView content={dagContent} />
						) : (
							<p className="text-sm text-muted-foreground">
								No DAG yet for this conversation.
							</p>
						)}
					</CardContent>
				)}
			</div>

			<Card>
				<CardHeader>
					<div
						role="button"
						tabIndex={0}
						aria-expanded={actionListExpanded}
						className="flex cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						onClick={() => setActionListExpanded((prev) => !prev)}
						onKeyDown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault()
								setActionListExpanded((prev) => !prev)
							}
						}}
					>
						{actionListExpanded ? (
							<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
						) : (
							<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
						)}
						<CardTitle>Action List</CardTitle>
					</div>
				</CardHeader>
				{actionListExpanded && (
					<CardContent className="space-y-4">
					{showProcessPlanHero ? (
						<div className="flex flex-col items-center gap-3 py-2">
							<div className="flex items-center justify-center gap-2">
								<Button
									type="button"
									size="lg"
									className={cn(
										'h-12 min-w-[16rem] px-8 text-base font-semibold',
										'bg-success text-success-foreground hover:bg-success/90',
										'elev-3 shadow-[0_0_0_4px_var(--success-border)]',
									)}
									onClick={() => void handleProcessPlan()}
									disabled={processPlanDisabled}
								>
									Process plan here
									<ArrowDown className="h-5 w-5" />
								</Button>
							</div>
						</div>
					) : (
						<div
							className={cn(
								'flex items-center gap-2',
								(processingPlan || fanoutRunning) && 'justify-center',
							)}
						>
							<Button
								type="button"
								onClick={() => void handleProcessPlan()}
								disabled={processPlanDisabled}
							>
								{fanoutRunning
									? 'Planning stages...'
									: processingPlan
										? 'Starting...'
										: 'Replan all stages'}
							</Button>
							{fanoutRunning && (
								<Button
									type="button"
									variant="outline"
									onClick={() => void handleCancelFanout()}
								>
									Stop
								</Button>
							)}
						</div>
					)}

					{!canProcessPlan && (
						<p className="text-sm text-muted-foreground">
							Complete decomposition first — a DAG is required before
							processing the plan.
						</p>
					)}

					{fanoutRun && fanoutRun.stages.length > 0 && (
						<div className="space-y-1 rounded-md border p-3">
							<div
								role="button"
								tabIndex={0}
								aria-expanded={stagesExpanded}
								className="flex cursor-pointer items-center gap-2 rounded-sm text-sm font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring"
								onClick={() => setStagesExpanded((prev) => !prev)}
								onKeyDown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault()
										setStagesExpanded((prev) => !prev)
									}
								}}
							>
								{stagesExpanded ? (
									<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
								) : (
									<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
								)}
								Stages{' '}
								<span className="text-muted-foreground">
									(
									{
										fanoutRun.stages.filter((st) => st.status === 'done')
											.length
									}
									/{fanoutRun.stages.length})
								</span>
							</div>
							{stagesExpanded && (
								<>
									{fanoutRun.stages.map((stage) => (
										<div
											key={`${stage.kind ?? 'stage'}:${stage.title}`}
											className="flex items-center justify-between gap-2 text-sm"
										>
											<span className="truncate">{stage.title}</span>
											<span className="flex shrink-0 items-center gap-2">
												<span
													className={
														stage.status === 'failed'
															? 'text-destructive'
															: stage.status === 'done'
																? 'text-lime-700 dark:text-lime-300'
																: ''
													}
													title={stage.error}
												>
													{stage.status}
												</span>
												{stage.dialogId && (
													<Button
														type="button"
														variant="ghost"
														size="sm"
														onClick={() =>
															handleOpenStageDialog(stage.dialogId as string)
														}
													>
														Open
													</Button>
												)}
												{stage.kind !== 'rollback' &&
													!fanoutRunning &&
													dialog.mode === 'main' && (
														<Button
															type="button"
															variant="ghost"
															size="sm"
															onClick={() => handleReplanStage(stage.title)}
														>
															Replan
														</Button>
													)}
											</span>
										</div>
									))}
									{fanoutRun.status === 'awaiting_input' && (
										<p className="pt-2 text-sm text-muted-foreground">
											Some stages rest on assumptions. Answer the questions in
											the chat and those stages are replanned.
										</p>
									)}
								</>
							)}
						</div>
					)}

					{actionPlan ? (
						<ActionPlanView
							plan={actionPlan}
							checked={actionPlanChecked}
							comments={actionPlanComments}
							onToggle={handleActionPlanToggle}
							onComment={handleOpenComment}
							onEdit={handleOpenEditAction}
							onExecute={handleExecuteAction}
							onRestart={handleRestartAction}
							onStop={() => void handleStopExecution()}
							execRuns={execRuns}
							onReorder={(scope, stage, from, to) =>
								void handleReorder(scope, stage, from, to)
							}
							reordering={reordering}
							executorDisabled={executorType === 'disabled'}
						/>
					) : fanoutRun ? (
						<p className="text-sm text-muted-foreground">
							No stages stored yet.
						</p>
					) : null}
					</CardContent>
				)}
			</Card>

			<Dialog
				open={commentDialogKey !== null}
				onOpenChange={(open) => {
					if (!open) handleCloseComment()
				}}
			>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Action comment</DialogTitle>
					</DialogHeader>
					<Textarea
						value={commentDraft}
						onChange={(e) => setCommentDraft(e.target.value)}
						onKeyDown={(e) => {
							if (e.key === 'Escape') {
								e.preventDefault()
								handleCloseComment()
							}
							if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
								e.preventDefault()
								void handleSaveComment()
							}
						}}
						disabled={savingComment}
						className="min-h-[120px] whitespace-pre-wrap"
						aria-label="Action comment"
						placeholder="Add a note for this action..."
					/>
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={handleCloseComment}
							disabled={savingComment}
						>
							Cancel
						</Button>
						<Button
							type="button"
							onClick={() => void handleSaveComment()}
							disabled={savingComment}
						>
							{savingComment ? 'Saving...' : 'Save'}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>

			<Dialog
				open={editActionKey !== null}
				onOpenChange={(open) => {
					if (!open) handleCloseEditAction()
				}}
			>
				<DialogContent>
					<DialogHeader>
						<DialogTitle>Edit action</DialogTitle>
					</DialogHeader>
					<Textarea
						value={editActionDraft}
						onChange={(e) => setEditActionDraft(e.target.value)}
						onKeyDown={handleEditActionKeyDown}
						disabled={savingAction}
						className="min-h-[120px] whitespace-pre-wrap"
						aria-label="Action description"
						placeholder="Describe this action..."
					/>
					{editingStepHasCommand ? (
						<div className="space-y-1.5">
							<p className="text-xs font-medium text-muted-foreground">
								Command
							</p>
							<Textarea
								value={editCommandDraft}
								onChange={(e) => setEditCommandDraft(e.target.value)}
								onKeyDown={handleEditActionKeyDown}
								disabled={savingAction}
								className="min-h-[80px] overflow-x-auto whitespace-pre font-mono text-xs"
								aria-label="Action command"
								placeholder="kubectl -n prod rollout status deployment/api"
								spellCheck={false}
							/>
						</div>
					) : null}
					<DialogFooter>
						<Button
							type="button"
							variant="outline"
							onClick={handleCloseEditAction}
							disabled={savingAction}
						>
							Cancel
						</Button>
						<Button
							type="button"
							onClick={() => void handleSaveAction()}
							disabled={savingAction}
						>
							{savingAction ? 'Saving...' : 'Save'}
						</Button>
					</DialogFooter>
				</DialogContent>
			</Dialog>
		</div>
	)
}
