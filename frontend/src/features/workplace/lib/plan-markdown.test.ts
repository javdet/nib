import { describe, expect, it } from 'vitest'
import type { ActionPlan } from '@/features/dialogs/api/dialogs'
import {
	buildPlanMarkdown,
	planMarkdownFileName,
	type PlanExportInput,
} from './plan-markdown'

const baseInput: PlanExportInput = {
	title: 'Deploy API',
	planStatus: 'in_progress',
	scheduledAt: 1_704_067_200,
	createdAt: '2024-01-01T09:00:00.000Z',
	updatedAt: '2024-01-02T10:30:00.000Z',
	summary: 'Roll out the API deployment to production.',
	dag: '```mermaid\ngraph TD\n  A --> B\n```',
	plan: null,
	checked: [],
	comments: {},
}

const fullPlan: ActionPlan = {
	stages: [
		{
			number: 1,
			title: 'Deploy',
			description: 'Deploy the service.',
			steps: [
				{
					type: 'shell',
					action: 'Run rollout status',
					command: 'kubectl rollout status deployment/api',
				},
				{
					type: 'code',
					action: 'Open pull request',
					repository: 'org/api',
					pr_title: 'feat: deploy api',
					pr_url: 'https://github.com/org/api/pull/1',
				},
			],
			checks: [
				{
					check: 'Pods are healthy',
					expectation: 'All pods report Ready',
				},
			],
		},
	],
	rollback: [
		{
			type: 'shell',
			action: 'Rollback deployment',
			command: 'kubectl rollout undo deployment/api',
		},
	],
}

describe('buildPlanMarkdown', () => {
	it('renders a full plan with checks, comments, and commands', () => {
		const markdown = buildPlanMarkdown({
			...baseInput,
			plan: fullPlan,
			checked: ['s0.step0', 's0.check0'],
			comments: {
				's0.step0': 'Run during maintenance window',
				'rollback.0': 'Only if health checks fail',
			},
		})

		expect(markdown).toContain('# Deploy API')
		expect(markdown).toContain('## Status')
		expect(markdown).toContain('IN PROGRESS')
		expect(markdown).toContain('## Summary')
		expect(markdown).toContain('Roll out the API deployment to production.')
		expect(markdown).toContain('## DAG')
		expect(markdown).toContain('graph TD')
		expect(markdown).toContain('### Stage 1. Deploy')
		expect(markdown).toContain('- [x] **1.1** Action')
		expect(markdown).toContain('- [ ] **1.2** Action')
		expect(markdown).toContain('**Type:** shell')
		expect(markdown).toContain('kubectl rollout status deployment/api')
		expect(markdown).toContain('**Repository:** org/api')
		expect(markdown).toContain('https://github.com/org/api/pull/1')
		expect(markdown).toContain('**Comment:** Run during maintenance window')
		expect(markdown).toContain('#### Checks')
		expect(markdown).toContain('- [x] **1.C1** Pods are healthy')
		expect(markdown).toContain('**Expectation:** All pods report Ready')
		expect(markdown).toContain('### Rollback')
		expect(markdown).toContain('- [ ] **R1** Action')
		expect(markdown).toContain('**Comment:** Only if health checks fail')
	})

	it('uses placeholders when the action plan is missing', () => {
		const markdown = buildPlanMarkdown({
			...baseInput,
			summary: null,
			dag: null,
			plan: null,
		})

		expect(markdown).toContain('_Not set yet._')
		expect(markdown).toContain('## Action List')
	})

	it('shows an unscheduled date when scheduledAt is zero', () => {
		const markdown = buildPlanMarkdown({
			...baseInput,
			scheduledAt: 0,
		})

		expect(markdown).toContain('**Scheduled:** Not scheduled')
	})

	it('indents multi-line markdown actions for valid export nesting', () => {
		const markdown = buildPlanMarkdown({
			...baseInput,
			plan: {
				stages: [
					{
						number: 1,
						title: 'Update infra',
						description: '',
						steps: [
							{
								type: 'code',
								action:
									'All repository changes in one pull request.\n\n- `terraform/stage/main.tf`: bump `version` to `1.33.9`\n- `terraform/prod/main.tf`: bump `version` to `1.33.9`',
								repository: 'org/infra',
								pr_title: 'feat: bump cluster version',
							},
						],
						checks: [],
					},
				],
				rollback: [],
			},
		})

		expect(markdown).toContain(
			'  - All repository changes in one pull request.',
		)
		expect(markdown).toContain(
			'    - `terraform/stage/main.tf`: bump `version` to `1.33.9`',
		)
		expect(markdown).toContain(
			'    - `terraform/prod/main.tf`: bump `version` to `1.33.9`',
		)
	})
})

describe('planMarkdownFileName', () => {
	it('sanitizes the title for download', () => {
		expect(planMarkdownFileName('Deploy API v2!')).toBe('deploy-api-v2.md')
	})

	it('falls back to plan when the title is empty', () => {
		expect(planMarkdownFileName('   ')).toBe('plan.md')
	})
})
