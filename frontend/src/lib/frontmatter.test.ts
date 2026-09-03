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
})
