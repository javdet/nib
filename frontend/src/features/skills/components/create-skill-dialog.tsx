import { useState, useEffect } from 'react'
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogDescription,
	DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { buildContent } from '@/lib/frontmatter'

const NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/

interface CreateSkillDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onCreate: (name: string, content: string) => void
	existingNames: string[]
	creating?: boolean
}

export function CreateSkillDialog({
	open,
	onOpenChange,
	onCreate,
	existingNames,
	creating = false,
}: CreateSkillDialogProps) {
	const [name, setName] = useState('')
	const [validationError, setValidationError] = useState<string | null>(null)

	useEffect(() => {
		if (open) {
			setName('')
			setValidationError(null)
		}
	}, [open])

	function handleCreate() {
		const trimmed = name.trim()
		if (!trimmed) {
			setValidationError('Name is required')
			return
		}
		if (!NAME_PATTERN.test(trimmed)) {
			setValidationError(
				'Use letters, numbers, hyphens, and underscores; start with a letter or number',
			)
			return
		}
		if (existingNames.includes(trimmed)) {
			setValidationError('A skill with this name already exists')
			return
		}
		const content = buildContent({
			name: trimmed,
			description: 'Short description of when to use this skill',
			body: 'Step-by-step instructions...',
		})
		onCreate(trimmed, content)
	}

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!creating) onOpenChange(next)
			}}
		>
			<DialogContent className="sm:max-w-md">
				<DialogHeader>
					<DialogTitle>Add skill</DialogTitle>
					<DialogDescription>
						Create a new markdown skill file. The name becomes{' '}
						<span className="font-mono">name.md</span> on disk.
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-2">
					<Label htmlFor="skill-file-name">File name</Label>
					<Input
						id="skill-file-name"
						value={name}
						disabled={creating}
						onChange={(e) => {
							setName(e.target.value)
							setValidationError(null)
						}}
						placeholder="sentry-kafka-cleanup"
						className="font-mono"
						onKeyDown={(e) => {
							if (e.key === 'Enter') {
								e.preventDefault()
								handleCreate()
							}
						}}
					/>
					{validationError && (
						<p className="text-sm text-destructive">{validationError}</p>
					)}
				</div>

				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						onClick={() => onOpenChange(false)}
						disabled={creating}
					>
						Cancel
					</Button>
					<Button
						type="button"
						onClick={handleCreate}
						disabled={!name.trim() || creating}
					>
						{creating ? 'Creating...' : 'Create'}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
