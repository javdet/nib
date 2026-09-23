import { describe, expect, it } from 'vitest'
import { buildContent, parseFrontmatter } from './frontmatter'

const skillSample = `---
name: sentry-kafka-cleanup
description: Cleanup kafka topic
---

docker compose down
docker volume rm sentry-kafka && docker volume create sentry-kafka
`

const ruleNoBody = `---
name: Vault
description: Kubernetes global important rules
---`

describe('parseFrontmatter', () => {
	it('parses skill file with body', () => {
		const parsed = parseFrontmatter(skillSample)
		expect(parsed.name).toBe('sentry-kafka-cleanup')
		expect(parsed.description).toBe('Cleanup kafka topic')
		expect(parsed.body).toContain('docker compose down')
	})

	it('parses rule file without body', () => {
		const parsed = parseFrontmatter(ruleNoBody)
		expect(parsed.name).toBe('Vault')
		expect(parsed.description).toBe('Kubernetes global important rules')
		expect(parsed.body).toBe('')
	})

	it('round-trips skill content', () => {
		const parsed = parseFrontmatter(skillSample)
		const rebuilt = buildContent(parsed)
		const reparsed = parseFrontmatter(rebuilt)
		expect(reparsed).toEqual(parsed)
	})

	it('round-trips body-less rule content', () => {
		const parsed = parseFrontmatter(ruleNoBody)
		const rebuilt = buildContent(parsed)
		expect(rebuilt).toBe(ruleNoBody)
	})

	it('drops an unknown frontmatter key on round-trip', () => {
		const content = `---
name: jira-todo-tasks
description: Retrieve todo tasks from Jira
category: included
---

body`
		const parsed = parseFrontmatter(content)
		expect(parsed.name).toBe('jira-todo-tasks')
		const rebuilt = buildContent(parsed)
		expect(rebuilt).not.toContain('category:')
	})

	it('reads the access setting of a restricted skill', () => {
		const content = `---
name: get-jira-task
description: Fetch one task
access: explicit
---

body`
		expect(parseFrontmatter(content).access).toBe('explicit')
		expect(buildContent(parseFrontmatter(content))).toBe(content)
	})

	it('treats an unknown access value as enabled', () => {
		const content = `---
name: a
description: b
access: sometimes
---`
		expect(parseFrontmatter(content).access).toBe('enabled')
		expect(buildContent(parseFrontmatter(content))).not.toContain('access:')
	})

	it('never writes access: enabled', () => {
		const parsed = parseFrontmatter(skillSample)
		expect(parsed.access).toBeUndefined()
		expect(buildContent({ ...parsed, access: 'enabled' })).toBe(
			buildContent(parsed),
		)
		expect(buildContent({ ...parsed, access: 'disabled' })).toContain(
			'\naccess: disabled\n---',
		)
	})
})
