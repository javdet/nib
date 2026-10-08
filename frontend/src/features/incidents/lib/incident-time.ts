import { format } from 'date-fns'

function parse(value: string): Date | null {
	if (!value) return null
	const date = new Date(value)
	return Number.isNaN(date.getTime()) ? null : date
}

/** Renders a stored time for reading; an unparseable one is shown as written. */
export function formatIncidentTime(value: string): string {
	const date = parse(value)
	if (!date) return value
	return format(date, 'PPP HH:mm')
}

/**
 * Converts a stored RFC 3339 time to the value of a `datetime-local` input,
 * which is local time without a zone.
 */
export function toLocalInputValue(value: string): string {
	const date = parse(value)
	if (!date) return ''
	return format(date, "yyyy-MM-dd'T'HH:mm")
}

/** Converts a `datetime-local` value back to RFC 3339 UTC, or '' when empty. */
export function fromLocalInputValue(value: string): string {
	if (!value) return ''
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) return ''
	return date.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/** Human duration between two stored times, or null when either is missing. */
export function incidentDuration(from: string, to: string): string | null {
	const start = parse(from)
	const end = parse(to)
	if (!start || !end || end < start) return null
	const minutes = Math.round((end.getTime() - start.getTime()) / 60_000)
	if (minutes < 60) return `${minutes} min`
	const hours = Math.floor(minutes / 60)
	const rest = minutes % 60
	if (hours < 48) return rest ? `${hours} h ${rest} min` : `${hours} h`
	const days = Math.floor(hours / 24)
	const restHours = hours % 24
	return restHours ? `${days} d ${restHours} h` : `${days} d`
}
