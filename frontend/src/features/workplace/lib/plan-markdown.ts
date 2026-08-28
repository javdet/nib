import { format } from 'date-fns'
import type {
	ActionCheck,
	ActionPlan,
	ActionStep,
	PlanStatus,
} from '@/features/dialogs/api/dialogs'
import { getPlanStatusLabel } from '../components/plan-status-badge'

export interface PlanExportInput {
	title: string
	planStatus: PlanStatus
	scheduledAt: number
	createdAt: string
	updatedAt: string
	summary: string | null
	dag: string | null
	plan: ActionPlan | null
	checked: string[]
	comments: Record<string, string>
}

const PLACEHOLDER = '_Not set yet._'

function formatTimestamp(value: string): string {
	const date = new Date(value)
	if (Number.isNaN(date.getTime())) return value
	return format(date, 'PPP p')
}

function formatScheduledAt(unix: number): string {
	if (unix <= 0) return 'Not scheduled'
	return format(new Date(unix * 1000), 'PPP p')
}

function isChecked(checked: Set<string>, key: string): boolean {
	return checked.has(key)
}

function checkboxPrefix(checked: Set<string>, key: string): string {
	return isChecked(checked, key) ? '- [x]' : '- [ ]'
}

function appendLabels(lines: string[], step: ActionStep): void {
	const type = step.type?.trim()
	const repository = step.repository?.trim()
	const showRepository = type?.toLowerCase() === 'code' && !!repository

	if (type) {
		lines.push(`  - **Type:** ${type}`)
	}
	if (showRepository) {
		lines.push(`  - **Repository:** ${repository}`)
	}
}

function appendComment(lines: string[], comment: string | undefined): void {
	const trimmed = comment?.trim()
	if (!trimmed) return
	lines.push(`  - **Comment:** ${trimmed}`)
}

function appendNestedListItem(lines: string[], text: string, indent: string): void {
	const trimmed = text.trim()
	if (!trimmed) return

	const contentLines = trimmed.split('\n')
	lines.push(`${indent}- ${contentLines[0]}`)
	for (const line of contentLines.slice(1)) {
		lines.push(`${indent}  ${line}`)
	}
}

function appendStepDetails(
	lines: string[],
	step: ActionStep,
	comment: string | undefined,
): void {
	appendLabels(lines, step)

	const action = step.action?.trim()
	if (action) {
		appendNestedListItem(lines, action, '  ')
	}

	const isCode = step.type?.trim().toLowerCase() === 'code'
	const prTitle = step.pr_title?.trim()
	const prURL = step.pr_url?.trim()

	if (isCode) {
		if (prTitle) {
			lines.push(`  - **PR Title:** ${prTitle}`)
		}
		if (prURL) {
			lines.push(`  - **Pull request:** ${prURL}`)
		} else if (!prTitle) {
			lines.push(`  - **Pull request:** —`)
		}
	}

	const command = step.command?.trim()
	if (command) {
		lines.push('  - **Command:**')
		lines.push('    ```')
		lines.push(`    ${command}`)
		lines.push('    ```')
	}

	appendComment(lines, comment)
}

function renderStep(
	lines: string[],
	key: string,
	step: ActionStep,
	checked: Set<string>,
	comments: Record<string, string>,
): void {
	lines.push(`${checkboxPrefix(checked, key)} Action`)
	appendStepDetails(lines, step, comments[key])
}

function renderCheck(
	lines: string[],
	key: string,
	check: ActionCheck,
	checked: Set<string>,
): void {
	lines.push(`${checkboxPrefix(checked, key)} ${check.check.split('\n')[0]}`)
	const checkLines = check.check.trim().split('\n')
	if (checkLines.length > 1) {
		for (const line of checkLines.slice(1)) {
			lines.push(`  ${line}`)
		}
	}
	const expectation = check.expectation?.trim()
	if (expectation) {
		appendNestedListItem(lines, `**Expectation:** ${expectation}`, '  ')
	}
}

function renderActionList(
	lines: string[],
	plan: ActionPlan,
	checked: Set<string>,
	comments: Record<string, string>,
): void {
	for (const [stageIdx, stage] of plan.stages.entries()) {
		const stageNumber = stage.number || stageIdx + 1
		lines.push(`### Stage ${stageNumber}. ${stage.title}`)

		const description = stage.description?.trim()
		if (description) {
			lines.push('')
			lines.push(description)
		}

		if (stage.steps.length > 0) {
			lines.push('')
			for (const [stepIdx, step] of stage.steps.entries()) {
				const key = `s${stageIdx}.step${stepIdx}`
				renderStep(lines, key, step, checked, comments)
				lines.push('')
			}
		}

		if (stage.checks.length > 0) {
			lines.push('#### Checks')
			lines.push('')
			for (const [checkIdx, check] of stage.checks.entries()) {
				const key = `s${stageIdx}.check${checkIdx}`
				renderCheck(lines, key, check, checked)
				lines.push('')
			}
		}
	}

	if (plan.rollback.length > 0) {
		lines.push('### Rollback')
		lines.push('')
		for (const [idx, step] of plan.rollback.entries()) {
			const key = `rollback.${idx}`
			renderStep(lines, key, step, checked, comments)
			lines.push('')
		}
	}
}

export function buildPlanMarkdown(input: PlanExportInput): string {
	const checked = new Set(input.checked)
	const lines: string[] = []

	lines.push(`# ${input.title}`)
	lines.push('')
	lines.push('## Status')
	lines.push('')
	lines.push(getPlanStatusLabel(input.planStatus))
	lines.push('')
	lines.push('## Date')
	lines.push('')
	lines.push(`- **Scheduled:** ${formatScheduledAt(input.scheduledAt)}`)
	lines.push(`- **Created:** ${formatTimestamp(input.createdAt)}`)
	lines.push(`- **Updated:** ${formatTimestamp(input.updatedAt)}`)
	lines.push('')
	lines.push('## Summary')
	lines.push('')
	lines.push(input.summary?.trim() || PLACEHOLDER)
	lines.push('')
	lines.push('## DAG')
	lines.push('')
	lines.push(input.dag?.trim() || PLACEHOLDER)
	lines.push('')
	lines.push('## Action List')
	lines.push('')

	if (input.plan) {
		renderActionList(lines, input.plan, checked, input.comments)
	} else {
		lines.push(PLACEHOLDER)
	}

	return lines.join('\n').replace(/\n{3,}/g, '\n\n').trimEnd() + '\n'
}

export function planMarkdownFileName(title: string): string {
	const sanitized = title
		.trim()
		.toLowerCase()
		.replace(/[^\w\s-]/g, '')
		.replace(/\s+/g, '-')
		.replace(/-+/g, '-')
		.replace(/^-|-$/g, '')

	const base = sanitized || 'plan'
	return `${base}.md`
}
