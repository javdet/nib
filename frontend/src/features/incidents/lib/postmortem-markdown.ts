import type { Postmortem } from '../api/incidents'
import { formatIncidentTime, incidentDuration } from './incident-time'
import {
	actionStatusLabel,
	findingStatusStyle,
	incidentStatusStyle,
	severityStyle,
} from './postmortem-labels'

const PLACEHOLDER = '_Not established yet._'

function section(lines: string[], title: string, body: string): void {
	lines.push(`## ${title}`, '', body.trim() || PLACEHOLDER, '')
}

function escapeCell(value: string): string {
	return value.replace(/\|/g, '\\|').replace(/\n+/g, ' ')
}

/** Renders the post-mortem as a standalone Markdown document. */
export function buildPostmortemMarkdown(title: string, doc: Postmortem): string {
	const lines: string[] = [`# Post-mortem: ${title || 'Untitled incident'}`, '']

	const meta: [string, string][] = [
		['Status', incidentStatusStyle(doc.status).label],
		['Severity', severityStyle(doc.severity)?.label ?? 'Not set'],
		['Environment', doc.environment || 'Not set'],
		['Started', doc.startedAt ? formatIncidentTime(doc.startedAt) : 'Unknown'],
		['Detected', doc.detectedAt ? formatIncidentTime(doc.detectedAt) : 'Unknown'],
	]
	if (doc.mitigatedAt) meta.push(['Mitigated', formatIncidentTime(doc.mitigatedAt)])
	if (doc.resolvedAt) meta.push(['Resolved', formatIncidentTime(doc.resolvedAt)])
	const duration = incidentDuration(doc.startedAt, doc.resolvedAt)
	if (duration) meta.push(['Duration', duration])
	for (const [label, value] of meta) {
		lines.push(`- **${label}:** ${value}`)
	}
	lines.push('')

	section(lines, 'Summary', doc.summary)
	section(lines, 'Working hypothesis', doc.hypothesis)

	lines.push('## Root cause', '')
	if (doc.rootCauses.length === 0) {
		lines.push(PLACEHOLDER)
	} else {
		doc.rootCauses.forEach((cause, i) => lines.push(`${i + 1}. ${cause.text}`))
	}
	lines.push('')

	lines.push('## Impacted areas', '')
	if (doc.impacted.length === 0) {
		lines.push(PLACEHOLDER)
	} else {
		for (const area of doc.impacted) {
			const env = area.environment ? ` (${area.environment})` : ''
			const severity = area.severity ? ` — ${area.severity}` : ''
			const description = area.description ? `: ${area.description}` : ''
			lines.push(`- **${area.name}**${env}${severity}${description}`)
		}
	}
	lines.push('')

	lines.push('## Timeline (UTC)', '')
	if (doc.timeline.length === 0) {
		lines.push(PLACEHOLDER)
	} else {
		lines.push('| Time | Event | Source |', '| --- | --- | --- |')
		for (const event of doc.timeline) {
			lines.push(
				`| ${escapeCell(event.at)} | ${escapeCell(event.event)} | ${escapeCell(event.source ?? '')} |`,
			)
		}
	}
	lines.push('')

	for (const [heading, actions] of [
		['Short-term changes', doc.shortTerm],
		['Long-term fixes', doc.longTerm],
	] as const) {
		lines.push(`## ${heading}`, '')
		if (actions.length === 0) {
			lines.push(PLACEHOLDER)
		} else {
			for (const action of actions) {
				const box = action.status === 'done' ? '[x]' : '[ ]'
				const status =
					action.status === 'todo' ? '' : ` _(${actionStatusLabel(action.status)})_`
				lines.push(`- ${box} ${action.text}${status}`)
			}
		}
		lines.push('')
	}

	if (doc.findings.length > 0) {
		lines.push('## Findings by system', '')
		for (const finding of doc.findings) {
			const env = finding.environment ? ` (${finding.environment})` : ''
			lines.push(
				`### ${finding.system}${env} — ${findingStatusStyle(finding.status).label}`,
				'',
				finding.summary.trim() || PLACEHOLDER,
				'',
			)
			if (finding.suspectedCause) {
				lines.push(`- **Suspected cause:** ${finding.suspectedCause}`)
			}
			if (finding.changes && finding.changes.length > 0) {
				lines.push(`- **Changes:** ${finding.changes.join('; ')}`)
			}
			if (finding.confidence) {
				lines.push(`- **Confidence:** ${finding.confidence}`)
			}
			lines.push('')
		}
	}

	section(lines, 'Impact', doc.impact)
	section(lines, 'Closure', doc.closure)

	return lines.join('\n').replace(/\n{3,}/g, '\n\n').trimEnd() + '\n'
}

/** File name for the downloaded post-mortem. */
export function postmortemMarkdownFileName(title: string): string {
	const slug = title
		.toLowerCase()
		.replace(/[^a-z0-9]+/g, '-')
		.replace(/^-+|-+$/g, '')
		.slice(0, 60)
	return `postmortem-${slug || 'incident'}.md`
}
