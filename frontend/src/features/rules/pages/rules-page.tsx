import { useMemo, useState } from 'react'
import { ShieldCheck } from 'lucide-react'
import { MarkdownResourcePage } from '@/components/markdown-resource-page'
import { useNamedMarkdownResource } from '@/lib/hooks/use-named-markdown-resource'
import { CreateRuleDialog } from '../components/create-rule-dialog'
import {
	listRules,
	getRule,
	createRule,
	updateRule,
	deleteRule,
} from '../api/rules'

export function RulesPage() {
	const [createOpen, setCreateOpen] = useState(false)

	const api = useMemo(
		() => ({
			list: async () => {
				const result = await listRules()
				return { names: result.rules.map((rule) => rule.name) }
			},
			get: async (name: string) => {
				const rule = await getRule(name)
				return { content: rule.content }
			},
			create: async (name: string, content: string) => {
				await createRule(name, content)
			},
			update: updateRule,
			delete: deleteRule,
		}),
		[],
	)

	const resource = useNamedMarkdownResource(api, 'rules')

	return (
		<MarkdownResourcePage
			title="Rules"
			description="Edit deployment and product rules. Each file holds guardrails for a specific service or product."
			listTitle="Rules"
			addLabel="Add rule"
			onAddClick={() => setCreateOpen(true)}
			emptyIcon={<ShieldCheck className="h-6 w-6" />}
			emptyListMessage="No rules yet."
			emptySelectionMessage="Select a rule or add a new one."
			loadingSelectionMessage="Loading rules..."
			resource={resource}
			createDialog={
				<CreateRuleDialog
					open={createOpen}
					onOpenChange={setCreateOpen}
					onCreate={async (name, content) => {
						await resource.create(name, content)
						setCreateOpen(false)
					}}
					existingNames={resource.names}
					creating={resource.saving && createOpen}
				/>
			}
		/>
	)
}
