import {
	useCallback,
	useEffect,
	useRef,
	useState,
	type KeyboardEvent,
} from 'react'
import { useNavigate, useParams } from 'react-router'
import { ArrowLeft, ChevronDown, ChevronRight, Download, Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { IconButton } from '@/components/ui/icon-button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
	Dialog,
	DialogContent,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { cn } from '@/lib/utils'
import { downloadTextFile } from '@/lib/download'
import {
	createDialog,
	executeActionPlanAction,
	ensureActionPlanExecutorChat,
	getDialog,
	getDialogActionPlan,
	getDialogDag,
	getDialogRules,
	getDialogSummary,
	getPlanState,
	listDialogChildren,
	openDialogActivity,
	updateActionPlan,
	updateActionPlanChecks,
	updateActionPlanComments,
	reorderActionPlanItems,
	updatePlanSchedule,
	updateDialogSummary,
	updateDialogTitle,
	updateDialogSubjects,
	updateDialogCategories,
	type ActionPlan,
	type ActionStep,
	type ActionPlanScope,
	type Dialog as DialogItem,
	type PlanStatus,
} from '@/features/dialogs/api/dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { dialogDisplayTitle } from '@/features/dialogs/lib/dialog-title'
import { useMode } from '@/features/modes/mode-context'
import { ActionPlanView } from '../components/action-plan-view'
import { DagView } from '../components/dag-view'
import { PlanMetaTable } from '../components/plan-meta-table'
import { PlanProgressBar } from '../components/plan-progress-bar'
import {
	buildPlanMarkdown,
	planMarkdownFileName,
} from '../lib/plan-markdown'

const summaryTextareaClasses = cn(
	'min-h-[120px] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2',
	'text-sm shadow-sm placeholder:text-muted-foreground',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
	'whitespace-pre-wrap',
)

const commandTextareaClasses = cn(
	'min-h-[80px] w-full resize-y overflow-x-auto rounded-md border border-input bg-transparent px-3 py-2',
	'font-mono text-xs shadow-sm placeholder:text-muted-foreground',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
	'whitespace-pre',
)

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
	const [planChild, setPlanChild] = useState<DialogItem | null>(null)
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
	const [processingPlan, setProcessingPlan] = useState(false)
	const [isEditingTitle, setIsEditingTitle] = useState(false)
	const [titleDraft, setTitleDraft] = useState('')
	const [savingTitle, setSavingTitle] = useState(false)
	const [isEditingSummary, setIsEditingSummary] = useState(false)
	const [summaryDraft, setSummaryDraft] = useState('')
	const [savingSummary, setSavingSummary] = useState(false)
	const [savingSubjects, setSavingSubjects] = useState(false)
	const [savingCategories, setSavingCategories] = useState(false)
	const [liveActionPlanVersion, setLiveActionPlanVersion] = useState(0)
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
			const [dialogData, summary, dag, children, planState] = await Promise.all([
				getDialog(dialogId),
				getDialogSummary(dialogId),
				getDialogDag(dialogId),
				listDialogChildren(dialogId),
				getPlanState(dialogId),
			])
			const child = children.find((item) => item.mode === 'plan') ?? null
			const actionPlanData = child
				? await getDialogActionPlan(child.id)
				: null
			setDialog(dialogData)
			setSummaryContent(summary)
			setDagContent(dag)
			setPlanChild(child)
			setActionPlan(actionPlanData?.plan ?? null)
			setActionPlanChecked(actionPlanData?.checked ?? [])
			setActionPlanComments(actionPlanData?.comments ?? {})
			setPlanStatus(planState.status)
			setPlanScheduledAt(planState.scheduledAt)
		} catch (err) {
			setDialog(null)
			setSummaryContent(null)
			setDagContent(null)
			setPlanChild(null)
			setActionPlan(null)
			setActionPlanChecked([])
			setActionPlanComments({})
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

	useEffect(() => {
		if (!id) return
		void loadDetail(id)
	}, [id, loadDetail, dialogsVersion])

	useEffect(() => {
		if (!planChild) return

		const close = openDialogActivity(planChild.id, (ev) => {
			if (ev.kind === 'action_plan_updated') {
				setLiveActionPlanVersion((v) => v + 1)
			}
		})

		return close
	}, [planChild])

	useEffect(() => {
		if (
			!planChild ||
			(actionPlanVersion === 0 && liveActionPlanVersion === 0)
		) {
			return
		}

		let cancelled = false
		void getDialogActionPlan(planChild.id)
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
	}, [planChild, actionPlanVersion, liveActionPlanVersion])

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

	const handleSaveSubjects = useCallback(
		async (nextSubjects: string[]) => {
			if (!id || !dialog || savingSubjects) return

			const previous = dialog.subjects ?? []
			const unchanged =
				previous.length === nextSubjects.length &&
				previous.every((subject, index) => subject === nextSubjects[index])
			if (unchanged) {
				return
			}

			setDialog((current) =>
				current ? { ...current, subjects: nextSubjects } : current,
			)
			setSavingSubjects(true)
			setError(null)
			try {
				const updated = await updateDialogSubjects(id, nextSubjects)
				setDialog(updated)
			} catch (err) {
				setDialog((current) =>
					current ? { ...current, subjects: previous } : current,
				)
				setError(
					err instanceof Error
						? err.message
						: 'Failed to update components',
				)
			} finally {
				setSavingSubjects(false)
			}
		},
		[id, dialog, savingSubjects],
	)

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
					err instanceof Error ? err.message : 'Failed to update rules',
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

	const handleOpenPlanDialog = useCallback(
		(planDialog: DialogItem) => {
			selectMode(planDialog.mode)
			setActiveDialogId(planDialog.id)
		},
		[selectMode, setActiveDialogId],
	)

	const handleActionPlanToggle = useCallback(
		(key: string, nextChecked: boolean) => {
			if (!planChild) return

			const prevChecked = actionPlanChecked
			const prevStatus = planStatus
			const next = nextChecked
				? [...actionPlanChecked, key]
				: actionPlanChecked.filter((item) => item !== key)

			setActionPlanChecked(next)

			// Status is derived from full plan progress on the server; taking it
			// from the response keeps a single source of truth.
			void updateActionPlanChecks(planChild.id, next)
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
		[planChild, actionPlanChecked, planStatus],
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
		if (!planChild || commentDialogKey === null || savingComment) return

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
			const saved = await updateActionPlanComments(planChild.id, next)
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
		planChild,
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
		if (!planChild || !actionPlan || editActionKey === null || savingAction) return

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
			const saved = await updateActionPlan(planChild.id, nextPlan)
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
		planChild,
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
			if (!planChild || reordering) return

			setReordering(true)
			setError(null)
			try {
				const result = await reorderActionPlanItems(
					planChild.id,
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
		[planChild, reordering],
	)

	const handleExecuteAction = useCallback(
		async (step: ActionStep, key: string) => {
			if (!planChild) return

			try {
				// A code action goes straight to the coding agent: the backend
				// creates the chat, stores the task in it, and launches the
				// container. No message is sent to the LLM.
				if (step.type === 'code') {
					const { dialog } = await executeActionPlanAction(
						planChild.id,
						key,
					)
					selectMode('execute')
					setActiveDialogId(dialog.id)
					bumpDialogsVersion()
					return
				}

				const { dialog, created } = await ensureActionPlanExecutorChat(
					planChild.id,
				)
				selectMode('execute')

				const parts: string[] = []
				if (created && summaryContent) {
					parts.push(`## Summary\n\n${summaryContent}`)
				}
				parts.push(`## Action Type\n\n${step.type}`)
				parts.push(`## Action\n\n${step.action}`)
				if (step.pr_title?.trim()) {
					parts.push(`## PR Title\n\n${step.pr_title.trim()}`)
				}
				const comment = actionPlanComments[key]?.trim()
				if (comment) {
					parts.push(`## Comment\n\n${comment}`)
				}
				enqueuePendingMessage({
					dialogId: dialog.id,
					text: parts.join('\n\n'),
				})

				setActiveDialogId(dialog.id)
				bumpDialogsVersion()
			} catch (err) {
				setError(
					err instanceof Error
						? err.message
						: 'Failed to start execution',
				)
			}
		},
		[
			planChild,
			summaryContent,
			actionPlanComments,
			selectMode,
			enqueuePendingMessage,
			setActiveDialogId,
			bumpDialogsVersion,
		],
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

	const handleProcessPlan = useCallback(async () => {
		if (!id) return

		if (planChild) {
			handleOpenPlanDialog(planChild)
			return
		}

		setProcessingPlan(true)
		try {
			const [created, matchedRules] = await Promise.all([
				createDialog({
					mode: 'plan',
					parentId: id,
				}),
				getDialogRules(id),
			])
			setPlanChild(created)
			selectMode('plan')

			const parts: string[] = []
			if (summaryContent) parts.push(`## Summary\n\n${summaryContent}`)
			if (dagContent) parts.push(`## DAG\n\n${dagContent}`)
			if (matchedRules.length > 0) {
				const rulesSection = matchedRules
					.map((rule) => `### ${rule.name}\n\n${rule.content}`)
					.join('\n\n')
				parts.push(`## Rules\n\n${rulesSection}`)
			}
			const firstMessage = parts.join('\n\n')
			if (firstMessage) {
				enqueuePendingMessage({ dialogId: created.id, text: firstMessage })
			}

			setActiveDialogId(created.id)
			bumpDialogsVersion()
		} catch (err) {
			setError(
				err instanceof Error
					? err.message
					: 'Failed to create plan dialog',
			)
		} finally {
			setProcessingPlan(false)
		}
	}, [
		id,
		planChild,
		summaryContent,
		dagContent,
		handleOpenPlanDialog,
		setActiveDialogId,
		enqueuePendingMessage,
		selectMode,
		bumpDialogsVersion,
	])

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
				<div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
					{error ?? 'Plan not found'}
				</div>
			</div>
		)
	}

	const canProcessPlan = Boolean(planChild || summaryContent || dagContent)
	const isDecomposed = Boolean(summaryContent && dagContent)

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
					components={dialog.subjects ?? []}
					rules={dialog.categories ?? []}
					onComponentsChange={(next) => void handleSaveSubjects(next)}
					onRulesChange={(next) => void handleSaveCategories(next)}
					disabled={savingSubjects || savingCategories}
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
						{!isEditingSummary && (
							<IconButton
								type="button"
								variant="ghost"
								className="ml-auto h-8 w-8 shrink-0 text-muted-foreground"
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
								<textarea
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
									className={summaryTextareaClasses}
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
							<p className="text-sm whitespace-pre-wrap">{summaryContent}</p>
						) : (
							<p className="text-sm text-muted-foreground">
								No summary yet for this conversation.
							</p>
						)}
					</CardContent>
				)}
			</Card>

			<Card>
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
			</Card>

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
					<Button
						type="button"
						onClick={() => void handleProcessPlan()}
						disabled={processingPlan || !canProcessPlan}
					>
						{processingPlan
							? 'Creating plan...'
							: planChild
								? 'Open chat'
								: 'Process Plan'}
					</Button>

					{!planChild && !canProcessPlan && (
						<p className="text-sm text-muted-foreground">
							Complete decomposition first — a summary or DAG is required
							before processing the plan.
						</p>
					)}

					{!planChild && canProcessPlan && (
						<p className="text-sm text-muted-foreground">
							No actions yet.
						</p>
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
							onReorder={(scope, stage, from, to) =>
								void handleReorder(scope, stage, from, to)
							}
							reordering={reordering}
						/>
					) : planChild ? (
						<p className="text-sm text-muted-foreground">
							Action plan not generated yet.
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
					<textarea
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
						className={summaryTextareaClasses}
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
					<textarea
						value={editActionDraft}
						onChange={(e) => setEditActionDraft(e.target.value)}
						onKeyDown={handleEditActionKeyDown}
						disabled={savingAction}
						className={summaryTextareaClasses}
						aria-label="Action description"
						placeholder="Describe this action..."
					/>
					{editingStepHasCommand ? (
						<div className="space-y-1.5">
							<p className="text-xs font-medium text-muted-foreground">
								Command
							</p>
							<textarea
								value={editCommandDraft}
								onChange={(e) => setEditCommandDraft(e.target.value)}
								onKeyDown={handleEditActionKeyDown}
								disabled={savingAction}
								className={commandTextareaClasses}
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
