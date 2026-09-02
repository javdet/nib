import { api, ApiError, apiUrl } from '@/lib/api-client'

export interface Attachment {
	id: string
	dialogId: string
	messageId?: number
	filename: string
	contentType: string
	kind: 'image' | 'text'
	sizeBytes: number
	url?: string
	createdAt: string
}

export interface Dialog {
	id: string
	title: string
	mode: string
	parentId?: string
	subjects?: string[]
	categories?: string[]
	pinned: boolean
	planStatus?: PlanStatus
	createdAt: string
	updatedAt: string
}

export interface StoredToolCall {
	id: string
	type?: string
	function: {
		name: string
		arguments: string
	}
}

export interface DialogMessage {
	role: string
	content: string
	toolCalls?: StoredToolCall[]
	toolCallId?: string
	name?: string
	attachments?: Attachment[]
}

export interface Question {
	question: string
	options?: string[]
}

export interface TableData {
	title?: string
	columns: string[]
	rows: string[][]
}

export interface CreateDialogInput {
	mode?: string
	title?: string
	parentId?: string
}

export interface DialogChatResponse {
	response?: string
	status?: 'awaiting_input'
	toolCallId?: string
	questions?: Question[]
	actionPlanUpdated?: boolean
}

export type AgentActivityKind =
	| 'tools_start'
	| 'tools_end'
	| 'turn_end'
	| 'agent_result'
	| 'action_plan_updated'
	| 'plan_stage_started'
	| 'plan_stage_done'
	| 'plan_stage_failed'
	| 'plan_fanout_done'
	| 'action_exec_started'
	| 'action_exec_done'
	| 'action_exec_failed'

export interface AgentActivity {
	kind: AgentActivityKind
	round?: number
	count?: number
	/** Plan stage a fan-out event belongs to. */
	stage?: string
	/** Row key of the action an action-execution event belongs to. */
	action?: string
	/** Outcome of a stage, a fan-out run, or an action run. */
	status?: string
}

export function openDialogActivity(
	id: string,
	onEvent: (ev: AgentActivity) => void,
): () => void {
	const source = new EventSource(
		apiUrl(`/dialogs/${encodeURIComponent(id)}/events`),
	)
	source.onmessage = (event) => {
		try {
			const parsed = JSON.parse(event.data) as AgentActivity
			onEvent(parsed)
		} catch {
			// Ignore malformed SSE payloads.
		}
	}
	return () => {
		source.close()
	}
}

export type PlanStatus =
	| 'draft'
	| 'scheduled'
	| 'in_progress'
	| 'done'
	| 'reopened'
	| 'rolled_back'

export interface ActionStep {
	// number is the operator-facing label the backend derives from the item's
	// position ("1.2", "R1"). The UI derives its own from position instead, so
	// plans stored before numbering existed still render numbered.
	number?: string
	type: string
	action: string
	command?: string
	repository?: string
	pr_title?: string
	pr_url?: string
	comment?: string
}

export interface ActionCheck {
	number?: string
	check: string
	expectation: string
}

export interface ActionStage {
	number: number
	title: string
	description: string
	steps: ActionStep[]
	checks: ActionCheck[]
}

export interface ActionPlan {
	stages: ActionStage[]
	rollback: ActionStep[]
}

export interface ActionPlanResponse {
	plan: ActionPlan
	checked: string[]
	comments: Record<string, string>
	planStatus?: PlanStatus
}

export interface PlanState {
	status: PlanStatus
	scheduledAt: number
}

export interface DialogRule {
	name: string
	content: string
}

export function listDialogs(): Promise<Dialog[]> {
	return api.get<Dialog[]>('/dialogs')
}

export interface PagedDialogs {
	items: Dialog[]
	total: number
}

export async function listDialogsPaged(
	page: number,
	limit: number,
	scope?: 'all' | 'pinned',
	mode?: string,
	search?: string,
): Promise<PagedDialogs> {
	const params = new URLSearchParams({
		page: String(page),
		limit: String(limit),
	})
	if (scope === 'all') {
		params.set('scope', 'all')
	}
	if (scope === 'pinned') {
		params.set('scope', 'pinned')
	}
	if (mode) {
		params.set('mode', mode)
	}
	if (search) {
		params.set('search', search)
	}

	const { data, total } = await api.getWithTotal<Dialog[]>(
		`/dialogs?${params}`,
	)
	return { items: data, total }
}

export function createDialog(
	input: CreateDialogInput = {},
): Promise<Dialog> {
	return api.post<Dialog>('/dialogs', input)
}

export function getDialog(id: string): Promise<Dialog> {
	return api.get<Dialog>(`/dialogs/${encodeURIComponent(id)}`)
}

export function updateDialogTitle(id: string, title: string): Promise<Dialog> {
	return api.put<Dialog>(`/dialogs/${encodeURIComponent(id)}/title`, { title })
}

export function updateDialogSubjects(
	id: string,
	subjects: string[],
): Promise<Dialog> {
	return api.put<Dialog>(`/dialogs/${encodeURIComponent(id)}/subjects`, {
		subjects,
	})
}

export function updateDialogCategories(
	id: string,
	categories: string[],
): Promise<Dialog> {
	return api.put<Dialog>(`/dialogs/${encodeURIComponent(id)}/categories`, {
		categories,
	})
}

export function setDialogPinned(id: string, pinned: boolean): Promise<Dialog> {
	return api.put<Dialog>(`/dialogs/${encodeURIComponent(id)}/pin`, { pinned })
}

export function listPinnedDialogs(): Promise<Dialog[]> {
	return listDialogsPaged(1, 100, 'pinned').then(({ items }) => items)
}

export function listDialogChildren(id: string): Promise<Dialog[]> {
	return api
		.get<Dialog[]>(`/dialogs/${encodeURIComponent(id)}/children`)
		.catch((err) => {
			if (err instanceof ApiError && err.status === 404) return []
			throw err
		})
}

export function getDialogMessages(id: string): Promise<DialogMessage[]> {
	return api.get<DialogMessage[]>(
		`/dialogs/${encodeURIComponent(id)}/messages`,
	)
}

export function sendDialogMessage(
	id: string,
	message: string,
	attachmentIds?: string[],
	signal?: AbortSignal,
): Promise<DialogChatResponse> {
	return api.post<DialogChatResponse>(
		`/dialogs/${encodeURIComponent(id)}/messages`,
		{ message, attachmentIds: attachmentIds ?? [] },
		signal,
	)
}

export function retryLastResponse(
	id: string,
	signal?: AbortSignal,
): Promise<DialogChatResponse> {
	return api.post<DialogChatResponse>(
		`/dialogs/${encodeURIComponent(id)}/messages/retry`,
		{},
		signal,
	)
}

export async function uploadAttachment(
	dialogId: string,
	file: File,
): Promise<Attachment> {
	const form = new FormData()
	form.append('file', file)
	return api.upload<Attachment>(
		`/dialogs/${encodeURIComponent(dialogId)}/attachments`,
		form,
	)
}

export function deleteAttachment(
	dialogId: string,
	attachmentId: string,
): Promise<void> {
	return api.delete<void>(
		`/dialogs/${encodeURIComponent(dialogId)}/attachments/${encodeURIComponent(attachmentId)}`,
	)
}

export function listAttachments(dialogId: string): Promise<Attachment[]> {
	return api.get<Attachment[]>(
		`/dialogs/${encodeURIComponent(dialogId)}/attachments`,
	)
}

export function submitToolResults(
	id: string,
	toolCallId: string,
	answers: string[],
	signal?: AbortSignal,
): Promise<DialogChatResponse> {
	return api.post<DialogChatResponse>(
		`/dialogs/${encodeURIComponent(id)}/tool-results`,
		{ toolCallId, answers },
		signal,
	)
}

export function deleteDialog(id: string): Promise<void> {
	return api.delete<void>(`/dialogs/${encodeURIComponent(id)}`)
}

export function getDialogDag(id: string): Promise<string | null> {
	return api
		.get<{ content: string }>(`/dialogs/${encodeURIComponent(id)}/dag`)
		.then((r) => r.content)
		.catch((err) => {
			if (err instanceof ApiError && err.status === 404) return null
			throw err
		})
}

export function getDialogSummary(id: string): Promise<string | null> {
	return api
		.get<{ content: string }>(`/dialogs/${encodeURIComponent(id)}/summary`)
		.then((r) => r.content)
		.catch((err) => {
			if (err instanceof ApiError && err.status === 404) return null
			throw err
		})
}

export function updateDialogSummary(
	id: string,
	content: string,
): Promise<string> {
	return api
		.put<{ content: string }>(`/dialogs/${encodeURIComponent(id)}/summary`, {
			summary: content,
		})
		.then((r) => r.content)
}

export function getDialogRules(id: string): Promise<DialogRule[]> {
	return api
		.get<{ rules: DialogRule[] }>(`/dialogs/${encodeURIComponent(id)}/rules`)
		.then((r) => r.rules ?? [])
		.catch((err) => {
			if (err instanceof ApiError && err.status === 404) return []
			throw err
		})
}

export function getDialogActionPlan(
	id: string,
): Promise<ActionPlanResponse | null> {
	return api
		.get<ActionPlanResponse>(
			`/dialogs/${encodeURIComponent(id)}/action-plan`,
		)
		.catch((err) => {
			if (err instanceof ApiError && err.status === 404) return null
			throw err
		})
}

export interface ActionPlanChecksResponse {
	checked: string[]
	planStatus: PlanStatus
}

export function updateActionPlanChecks(
	id: string,
	checked: string[],
): Promise<ActionPlanChecksResponse> {
	return api.put<ActionPlanChecksResponse>(
		`/dialogs/${encodeURIComponent(id)}/action-plan/checks`,
		{ checked },
	)
}

export function updateActionPlanComments(
	id: string,
	comments: Record<string, string>,
): Promise<Record<string, string>> {
	return api
		.put<{ comments: Record<string, string> }>(
			`/dialogs/${encodeURIComponent(id)}/action-plan/comments`,
			{ comments },
		)
		.then((r) => r.comments)
}

export interface UpdateActionPlanResponse {
	plan: ActionPlan
	planStatus: PlanStatus
}

export function updateActionPlan(
	id: string,
	plan: ActionPlan,
): Promise<UpdateActionPlanResponse> {
	return api.put<UpdateActionPlanResponse>(
		`/dialogs/${encodeURIComponent(id)}/action-plan`,
		{ plan },
	)
}

export interface ActionRunResult {
	jobName: string
	containerName: string
	containerId: string
	targetBranch: string
	repoUrl: string
	status: string
}

export interface ExecuteActionResponse {
	dialog: Dialog
	run: ActionRunResult
}

/**
 * Launches the coding agent for a single `code` action of the plan stored on
 * `id`. The backend creates the execute chat and persists the task in it, so
 * the caller only has to open the returned dialog.
 */
export function executeActionPlanAction(
	id: string,
	key: string,
): Promise<ExecuteActionResponse> {
	return api.post<ExecuteActionResponse>(
		`/dialogs/${encodeURIComponent(id)}/action-plan/execute`,
		{ key },
	)
}

/** Where a per-action sub-agent run ended up. */
export type ActionExecStatus =
	| 'running'
	| 'done'
	| 'failed'
	| 'blocked'
	| 'cancelled'

/**
 * One attempt at one action row. It carries neither the row's number nor its
 * dialog id: the number is derived from position and moves when a stage is
 * reordered, and the dialog is recorded with the agent-runner runs.
 */
export interface ActionExecRun {
	status: ActionExecStatus
	startedAt: number
	finishedAt?: number
	error?: string
	attempt: number
}

/** Latest sub-agent run per action row key. */
export type ActionExecRuns = Record<string, ActionExecRun>

export function getActionPlanExecRuns(id: string): Promise<ActionExecRuns> {
	return api.get<ActionExecRuns>(
		`/dialogs/${encodeURIComponent(id)}/action-plan/exec`,
	)
}

export type ActionPlanScope = 'steps' | 'checks'

export function reorderActionPlanItems(
	id: string,
	scope: ActionPlanScope,
	stage: number,
	from: number,
	to: number,
): Promise<ActionPlanResponse> {
	return api.put<ActionPlanResponse>(
		`/dialogs/${encodeURIComponent(id)}/action-plan/reorder`,
		{ scope, stage, from, to },
	)
}

export function getPlanState(id: string): Promise<PlanState> {
	return api.get<PlanState>(`/dialogs/${encodeURIComponent(id)}/plan-state`)
}

export function updatePlanSchedule(
	id: string,
	scheduledAt: number,
): Promise<PlanState> {
	return api.put<PlanState>(
		`/dialogs/${encodeURIComponent(id)}/plan-state/schedule`,
		{ scheduledAt },
	)
}

export type FanoutStageStatus = 'pending' | 'running' | 'done' | 'failed'

export type FanoutRunStatus =
	| 'running'
	| 'awaiting_input'
	| 'done'
	| 'failed'

export type FanoutStageKind = 'rollback'

export interface FanoutStage {
	title: string
	wave: number
	status: FanoutStageStatus
	/**
	 * Absent for a DAG stage. The rollback agent carries one, because it is
	 * addressed by kind and its title is only a label.
	 */
	kind?: FanoutStageKind
	/** The subagent's own dialog, so its research can be read back. */
	dialogId?: string
	error?: string
}

export interface PlanBlocker {
	stage: string
	kind?: FanoutStageKind
	question: string
	options?: string[]
	assumption?: string
	answer?: string
}

export interface FanoutRun {
	runId: string
	status: FanoutRunStatus
	startedAt: number
	finishedAt?: number
	stages: FanoutStage[]
	blockers?: PlanBlocker[]
	pendingAskId?: string
	pendingStages?: string[]
	/** Whether the run the pending answers start also redoes the rollback. */
	pendingRollback?: boolean
	error?: string
}

/**
 * Plans every DAG stage with one subagent each, then works out the plan's
 * rollback with one more. Answers 202 straight away: the run continues in the
 * background and reports over the dialog's SSE stream.
 *
 * Naming stages restricts the run to them and still redoes the rollback, since
 * the rollback follows whatever those stages end up saying. Pass rollback
 * explicitly to override that either way.
 */
export function startPlanFanout(
	id: string,
	stages?: string[],
	rollback?: boolean,
): Promise<FanoutRun> {
	return api.post<FanoutRun>(
		`/dialogs/${encodeURIComponent(id)}/plan-fanout`,
		{ stages: stages ?? [], ...(rollback === undefined ? {} : { rollback }) },
	)
}

/** Reads the current run so a reloaded page can re-attach to one still going. */
export function getPlanFanout(id: string): Promise<FanoutRun | null> {
	return api
		.get<FanoutRun | null>(`/dialogs/${encodeURIComponent(id)}/plan-fanout`)
		.catch((err) => {
			if (err instanceof ApiError && err.status === 404) return null
			throw err
		})
}

/** Stops a running fan-out. Stages already written stay on the plan. */
export function cancelPlanFanout(id: string): Promise<void> {
	return api.delete<void>(`/dialogs/${encodeURIComponent(id)}/plan-fanout`)
}
