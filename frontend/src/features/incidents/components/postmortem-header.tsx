import { useEffect, useState } from 'react'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { cn } from '@/lib/utils'
import type {
	Postmortem,
	PostmortemPatch,
	PostmortemScalarField,
} from '../api/incidents'
import {
	fromLocalInputValue,
	incidentDuration,
	toLocalInputValue,
} from '../lib/incident-time'
import {
	INCIDENT_SEVERITIES,
	INCIDENT_STATUS_STYLES,
	incidentStatusStyle,
	severityStyle,
} from '../lib/postmortem-labels'
import { OperatorLockBadge } from './operator-lock-badge'

interface PostmortemHeaderProps {
	postmortem: Postmortem
	onEdit: (patch: PostmortemPatch) => Promise<void>
}

const TIME_FIELDS: { key: PostmortemScalarField; label: string }[] = [
	{ key: 'startedAt', label: 'Started' },
	{ key: 'detectedAt', label: 'Detected' },
	{ key: 'mitigatedAt', label: 'Mitigated' },
	{ key: 'resolvedAt', label: 'Resolved' },
]

function Pill({ label, className }: { label: string; className: string }) {
	return (
		<span
			className={cn(
				'inline-flex shrink-0 items-center rounded-full border px-3 py-1',
				'text-xs font-semibold uppercase tracking-wider',
				className,
			)}
		>
			{label}
		</span>
	)
}

/**
 * A text-like field saved when it loses focus, so typing does not write the
 * document on every keystroke. It re-reads the stored value whenever an agent
 * changes it, but never while the operator is in the middle of an edit.
 */
function BlurField({
	value,
	type,
	placeholder,
	ariaLabel,
	onCommit,
}: {
	value: string
	type: 'text' | 'datetime-local'
	placeholder?: string
	ariaLabel: string
	onCommit: (value: string) => void
}) {
	const [draft, setDraft] = useState(value)
	const [focused, setFocused] = useState(false)

	useEffect(() => {
		if (!focused) setDraft(value)
	}, [value, focused])

	return (
		<Input
			type={type}
			value={draft}
			placeholder={placeholder}
			aria-label={ariaLabel}
			onFocus={() => setFocused(true)}
			onChange={(e) => setDraft(e.target.value)}
			onBlur={() => {
				setFocused(false)
				if (draft !== value) onCommit(draft)
			}}
			onKeyDown={(e) => {
				if (e.key === 'Enter') e.currentTarget.blur()
			}}
		/>
	)
}

export function PostmortemHeader({ postmortem, onEdit }: PostmortemHeaderProps) {
	const [error, setError] = useState<string | null>(null)
	const locked = (field: PostmortemScalarField) => postmortem.locks.includes(field)

	const save = (field: PostmortemScalarField, value: string) => {
		setError(null)
		onEdit({ set: { [field]: value } }).catch((err: unknown) => {
			setError(err instanceof Error ? err.message : 'Failed to save')
		})
	}

	const unlock = (field: PostmortemScalarField) => () => onEdit({ unlock: [field] })

	const label = (field: PostmortemScalarField, text: string) => (
		<span className="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
			{text}
			{locked(field) && <OperatorLockBadge compact onUnlock={unlock(field)} />}
		</span>
	)

	const statusStyle = incidentStatusStyle(postmortem.status)
	const sevStyle = severityStyle(postmortem.severity)
	const duration = incidentDuration(
		postmortem.startedAt,
		postmortem.resolvedAt || postmortem.mitigatedAt,
	)
	// Closing goes through Close incident, which writes the closure report; the
	// select offers it only to show, or reopen, an incident already closed.
	const statuses = (Object.keys(INCIDENT_STATUS_STYLES) as (keyof typeof INCIDENT_STATUS_STYLES)[])
		.filter((status) => status !== 'closed' || postmortem.status === 'closed')

	return (
		<Card>
			<CardContent className="space-y-4 pt-6">
				<div className="flex flex-wrap items-center gap-2">
					<Pill label={statusStyle.label} className={statusStyle.className} />
					{sevStyle && <Pill label={sevStyle.label} className={sevStyle.className} />}
					{postmortem.environment && (
						<span className="text-sm text-muted-foreground">{postmortem.environment}</span>
					)}
					{duration && (
						<span className="ml-auto text-sm text-muted-foreground">Duration: {duration}</span>
					)}
				</div>
				<div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
					<label className="space-y-1">
						{label('status', 'Status')}
						<Select
							value={postmortem.status}
							onChange={(e) => save('status', e.target.value)}
							aria-label="Status"
						>
							{statuses.map((status) => (
								<option key={status} value={status}>
									{INCIDENT_STATUS_STYLES[status].label.toLowerCase()}
								</option>
							))}
						</Select>
					</label>
					<label className="space-y-1">
						{label('severity', 'Severity')}
						<Select
							value={postmortem.severity}
							onChange={(e) => save('severity', e.target.value)}
							aria-label="Severity"
						>
							<option value="">not set</option>
							{INCIDENT_SEVERITIES.map((severity) => (
								<option key={severity} value={severity}>
									{severity}
								</option>
							))}
						</Select>
					</label>
					<label className="space-y-1">
						{label('environment', 'Environment')}
						<BlurField
							type="text"
							value={postmortem.environment}
							placeholder="e.g. production"
							ariaLabel="Environment"
							onCommit={(value) => save('environment', value.trim())}
						/>
					</label>
				</div>
				<div className="grid grid-cols-1 gap-3 sm:grid-cols-4">
					{TIME_FIELDS.map(({ key, label: text }) => (
						<label key={key} className="space-y-1">
							{label(key, text)}
							<BlurField
								type="datetime-local"
								value={toLocalInputValue(postmortem[key])}
								ariaLabel={text}
								onCommit={(value) => save(key, fromLocalInputValue(value))}
							/>
						</label>
					))}
				</div>
				{error && <p className="text-sm text-destructive">{error}</p>}
			</CardContent>
		</Card>
	)
}
