import { useMemo, useState } from 'react'
import { FileCode2 } from 'lucide-react'
import { MarkdownResourcePage } from '@/components/markdown-resource-page'
import { useNamedMarkdownResource } from '@/lib/hooks/use-named-markdown-resource'
import { CreateSkillDialog } from '../components/create-skill-dialog'
import {
	listSkills,
	getSkill,
	createSkill,
	updateSkill,
	deleteSkill,
} from '../api/skills'

export function SkillsPage() {
	const [createOpen, setCreateOpen] = useState(false)

	const api = useMemo(
		() => ({
			list: async () => {
				const result = await listSkills()
				return { names: result.skills.map((skill) => skill.name) }
			},
			get: async (name: string) => {
				const skill = await getSkill(name)
				return { content: skill.content }
			},
			create: async (name: string, content: string) => {
				await createSkill(name, content)
			},
			update: updateSkill,
			delete: deleteSkill,
		}),
		[],
	)

	const resource = useNamedMarkdownResource(api, 'skills')

	return (
		<MarkdownResourcePage
			title="Skills"
			description="Edit reusable skills for the assistant. Each file holds instructions for a specific task or workflow."
			listTitle="Skills"
			addLabel="Add skill"
			onAddClick={() => setCreateOpen(true)}
			emptyIcon={<FileCode2 className="h-6 w-6" />}
			emptyListMessage="No skills yet."
			emptySelectionMessage="Select a skill or add a new one."
			loadingSelectionMessage="Loading skills..."
			resource={resource}
			createDialog={
				<CreateSkillDialog
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
