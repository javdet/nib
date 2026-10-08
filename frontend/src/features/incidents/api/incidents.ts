import { api, ApiError } from '@/lib/api-client'

export type IncidentStatus = 'ongoing' | 'mitigated' | 'resolved' | 'closed'

export type IncidentSeverity = '' | 'sev1' | 'sev2' | 'sev3' | 'sev4'

export type PostmortemActionStatus = 'todo' | 'in_plan' | 'done' | 'dropped'

export interface PostmortemItem {
	id: string
	text: string
	operator?: boolean
}

export interface PostmortemImpacted {
	id: string
	name: string
	environment?: string
	description?: string
	severity?: string
	operator?: boolean
}

export interface PostmortemEvent {
	id: string
	at: string
	event: string
	source?: string
	operator?: boolean
}

export interface PostmortemAction {
	id: string
	text: string
	status: PostmortemActionStatus
	planId?: string
	operator?: boolean
}

export interface PostmortemFinding {
	system: string
	environment?: string
	status: string
	summary: string
	before?: string
	during?: string
	changes?: string[]
	suspectedCause?: string
	confidence?: string
	assumptions?: string
	dialogId?: string
	updatedAt: number
}

export interface PostmortemLinkedPlan {
	planId: string
	actionIds: string[]
	createdAt: number
}

export interface Postmortem {
	version: number
	status: IncidentStatus
	severity: IncidentSeverity
	environment: string
	startedAt: string
	detectedAt: string
	mitigatedAt: string
	resolvedAt: string
	closedAt: string
	summary: string
	hypothesis: string
	impact: string
	closure: string
	rootCauses: PostmortemItem[]
	impacted: PostmortemImpacted[]
	timeline: PostmortemEvent[]
	shortTerm: PostmortemAction[]
	longTerm: PostmortemAction[]
	findings: PostmortemFinding[]
	linkedPlans: PostmortemLinkedPlan[]
	/** Scalar fields the operator owns; agents cannot overwrite them. */
	locks: string[]
	updatedAt: number
}

export type PostmortemScalarField =
	| 'status'
	| 'severity'
	| 'environment'
	| 'startedAt'
	| 'detectedAt'
	| 'mitigatedAt'
	| 'resolvedAt'
	| 'closedAt'
	| 'summary'
	| 'hypothesis'
	| 'impact'
	| 'closure'

export type PostmortemListName =
	| 'rootCauses'
	| 'impacted'
	| 'timeline'
	| 'shortTerm'
	| 'longTerm'

export interface PostmortemItemFields {
	text?: string
	name?: string
	environment?: string
	description?: string
	severity?: string
	at?: string
	event?: string
	source?: string
	status?: string
}

export interface PostmortemItemOp {
	list: PostmortemListName
	op: 'add' | 'update' | 'delete'
	id?: string
	fields?: PostmortemItemFields
}

/**
 * An operator's edit. Lists change through item operations, never whole, so
 * saving cannot drop what an agent added since the page last loaded.
 */
export interface PostmortemPatch {
	set?: Partial<Record<PostmortemScalarField, string>>
	ops?: PostmortemItemOp[]
	unlock?: PostmortemScalarField[]
}

function postmortemPath(id: string): string {
	return `/dialogs/${encodeURIComponent(id)}/postmortem`
}

export function getPostmortem(id: string): Promise<Postmortem> {
	return api.get<Postmortem>(postmortemPath(id))
}

export function editPostmortem(
	id: string,
	patch: PostmortemPatch,
): Promise<Postmortem> {
	return api.put<Postmortem>(postmortemPath(id), patch)
}

export interface IncidentPlan {
	planId: string
	/** First message of the plan's chat, sent by the chat panel. */
	message: string
}

export function createPlanFromIncident(
	id: string,
	actionIds: string[],
): Promise<IncidentPlan> {
	return api.post<IncidentPlan>(`${postmortemPath(id)}/plan`, { actionIds })
}

export interface PlanOrigin {
	incidentId: string
	incidentTitle: string
	actionIds: string[]
	createdAt: number
}

/** The incident a plan came from, or null for a plan created any other way. */
export async function getPlanOrigin(planId: string): Promise<PlanOrigin | null> {
	try {
		return await api.get<PlanOrigin>(`/dialogs/${encodeURIComponent(planId)}/origin`)
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) return null
		throw err
	}
}

export type IncidentRunKind = 'investigation' | 'closure'
export type IncidentRunStatus = 'running' | 'done' | 'failed'
export type IncidentStepKind = 'digger' | 'synthesis' | 'impact' | 'closure'
export type IncidentStepStatus = 'pending' | 'running' | 'done' | 'failed'

export interface IncidentStep {
	kind: IncidentStepKind
	system?: string
	environment?: string
	categories?: string[]
	focus?: string
	status: IncidentStepStatus
	dialogId?: string
	error?: string
	startedAt?: number
	finishedAt?: number
}

/** The latest investigation or closure of an incident. */
export interface IncidentRun {
	runId: string
	kind: IncidentRunKind
	status: IncidentRunStatus
	startedAt: number
	finishedAt?: number
	brief: string
	windowStart?: string
	windowEnd?: string
	steps: IncidentStep[]
	error?: string
}

function investigationPath(id: string): string {
	return `/dialogs/${encodeURIComponent(id)}/investigation`
}

/** The incident's latest run, or null before its first. */
export async function getInvestigation(id: string): Promise<IncidentRun | null> {
	try {
		return await api.get<IncidentRun>(investigationPath(id))
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) return null
		throw err
	}
}

export function stopInvestigation(id: string): Promise<void> {
	return api.delete<void>(investigationPath(id))
}

/**
 * Starts closing the incident: the final impact, then the closure summary.
 * The incident is marked closed only once both are written.
 */
export function closeIncident(id: string, resolvedAt: string): Promise<IncidentRun> {
	return api.post<IncidentRun>(`${postmortemPath(id)}/close`, { resolvedAt })
}
