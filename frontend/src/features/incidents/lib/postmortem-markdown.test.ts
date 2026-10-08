import { describe, expect, it } from 'vitest'
import type { Postmortem } from '../api/incidents'
import {
	buildPostmortemMarkdown,
	postmortemMarkdownFileName,
} from './postmortem-markdown'

const empty: Postmortem = {
	version: 0,
	status: 'ongoing',
	severity: '',
	environment: '',
	startedAt: '',
	detectedAt: '',
	mitigatedAt: '',
	resolvedAt: '',
	closedAt: '',
	summary: '',
	hypothesis: '',
	impact: '',
	closure: '',
	rootCauses: [],
	impacted: [],
	timeline: [],
	shortTerm: [],
	longTerm: [],
	findings: [],
	linkedPlans: [],
	locks: [],
	updatedAt: 0,
}

describe('buildPostmortemMarkdown', () => {
	it('renders placeholders for an empty document', () => {
		const md = buildPostmortemMarkdown('', empty)
		expect(md).toContain('# Post-mortem: Untitled incident')
		expect(md).toContain('- **Status:** ONGOING')
		expect(md).toContain('- **Severity:** Not set')
		expect(md).toContain('## Root cause\n\n_Not established yet._')
		expect(md).not.toContain('## Findings by system')
		expect(md).not.toMatch(/\n{3,}/)
	})

	it('renders every section of a filled document', () => {
		const md = buildPostmortemMarkdown('API outage', {
			...empty,
			status: 'resolved',
			severity: 'sev2',
			environment: 'prod',
			startedAt: '2026-10-07T14:00:00Z',
			resolvedAt: '2026-10-07T15:30:00Z',
			summary: 'Checkout failed for 5% of users.',
			rootCauses: [{ id: 'rc-1', text: 'Connection pool exhausted' }],
			impacted: [
				{ id: 'im-1', name: 'checkout', environment: 'prod', severity: 'high' },
			],
			timeline: [
				{ id: 'tl-1', at: '2026-10-07T14:00:00Z', event: 'Deploy | v2', source: 'argo' },
			],
			shortTerm: [
				{ id: 'st-1', text: 'Roll back', status: 'done' },
				{ id: 'st-2', text: 'Raise pool size', status: 'in_plan', planId: 'p' },
			],
			longTerm: [{ id: 'lt-1', text: 'Alert on pool usage', status: 'todo' }],
			findings: [
				{
					system: 'api',
					status: 'root_cause_candidate',
					summary: 'Pool at 100%.',
					changes: ['v2 deployed'],
					updatedAt: 1,
				},
			],
		})

		expect(md).toContain('- **Severity:** SEV2')
		expect(md).toContain('- **Duration:** 1 h 30 min')
		expect(md).toContain('1. Connection pool exhausted')
		expect(md).toContain('- **checkout** (prod) — high')
		expect(md).toContain('| 2026-10-07T14:00:00Z | Deploy \\| v2 | argo |')
		expect(md).toContain('- [x] Roll back')
		expect(md).toContain('- [ ] Raise pool size _(In plan)_')
		expect(md).toContain('- [ ] Alert on pool usage\n')
		expect(md).toContain('### api — Root cause candidate')
		expect(md).toContain('- **Changes:** v2 deployed')
	})
})

describe('postmortemMarkdownFileName', () => {
	it('slugs the title', () => {
		expect(postmortemMarkdownFileName('API outage: EU / prod')).toBe(
			'postmortem-api-outage-eu-prod.md',
		)
		expect(postmortemMarkdownFileName('')).toBe('postmortem-incident.md')
	})
})
