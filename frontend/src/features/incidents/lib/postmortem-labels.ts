import type {
	IncidentSeverity,
	IncidentStatus,
	PostmortemActionStatus,
} from '../api/incidents'

interface Style {
	label: string
	className: string
}

export const INCIDENT_STATUS_STYLES: Record<IncidentStatus, Style> = {
	ongoing: {
		label: 'ONGOING',
		className:
			'border-red-500/30 bg-red-500/15 text-red-700 dark:text-red-300',
	},
	mitigated: {
		label: 'MITIGATED',
		className:
			'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300',
	},
	resolved: {
		label: 'RESOLVED',
		className:
			'border-lime-500/30 bg-lime-500/15 text-lime-700 dark:text-lime-300',
	},
	closed: {
		label: 'CLOSED',
		className:
			'border-zinc-500/30 bg-zinc-500/15 text-zinc-700 dark:text-zinc-300',
	},
}

export const INCIDENT_STATUSES: IncidentStatus[] = [
	'ongoing',
	'mitigated',
	'resolved',
	'closed',
]

export const INCIDENT_SEVERITIES: Exclude<IncidentSeverity, ''>[] = [
	'sev1',
	'sev2',
	'sev3',
	'sev4',
]

export const SEVERITY_STYLES: Record<Exclude<IncidentSeverity, ''>, Style> = {
	sev1: {
		label: 'SEV1',
		className:
			'border-red-600/40 bg-red-600/20 text-red-700 dark:text-red-300',
	},
	sev2: {
		label: 'SEV2',
		className:
			'border-orange-500/30 bg-orange-500/15 text-orange-700 dark:text-orange-300',
	},
	sev3: {
		label: 'SEV3',
		className:
			'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300',
	},
	sev4: {
		label: 'SEV4',
		className:
			'border-sky-500/30 bg-sky-500/15 text-sky-700 dark:text-sky-300',
	},
}

export function incidentStatusStyle(status: string): Style {
	return (
		INCIDENT_STATUS_STYLES[status as IncidentStatus] ??
		INCIDENT_STATUS_STYLES.ongoing
	)
}

export function severityStyle(severity: string): Style | null {
	return SEVERITY_STYLES[severity as Exclude<IncidentSeverity, ''>] ?? null
}

export const ACTION_STATUS_LABELS: Record<PostmortemActionStatus, string> = {
	todo: 'To do',
	in_plan: 'In plan',
	done: 'Done',
	dropped: 'Dropped',
}

export function actionStatusLabel(status: string): string {
	return ACTION_STATUS_LABELS[status as PostmortemActionStatus] ?? status
}

const UNKNOWN_FINDING: Style = {
	label: 'Unknown',
	className:
		'border-zinc-500/30 bg-zinc-500/15 text-zinc-700 dark:text-zinc-300',
}

const FINDING_STATUS_STYLES: Record<string, Style> = {
	root_cause_candidate: {
		label: 'Root cause candidate',
		className:
			'border-red-500/30 bg-red-500/15 text-red-700 dark:text-red-300',
	},
	degraded: {
		label: 'Degraded',
		className:
			'border-amber-500/30 bg-amber-500/15 text-amber-700 dark:text-amber-300',
	},
	healthy: {
		label: 'Healthy',
		className:
			'border-lime-500/30 bg-lime-500/15 text-lime-700 dark:text-lime-300',
	},
	unknown: UNKNOWN_FINDING,
}

export function findingStatusStyle(status: string): Style {
	return FINDING_STATUS_STYLES[status] ?? UNKNOWN_FINDING
}
