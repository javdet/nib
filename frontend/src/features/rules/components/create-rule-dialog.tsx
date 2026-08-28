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

const NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/

const DEFAULT_RULE_TEMPLATE = `---
name: rule name
description: short description
---

- Rule item 1
- Rule item 2
`

interface CreateRuleDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onCreate: (name: string, content: string) => void
	existingNames: string[]
	creating?: boolean
}

export function CreateRuleDialog({
	open,
	onOpenChange,
	onCreate,
	existingNames,
	creating = false,
}: CreateRuleDialogProps) {
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
			setValidationError('A rule with this name already exists')
			return
		}
		onCreate(trimmed, DEFAULT_RULE_TEMPLATE)
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
					<DialogTitle>Add rule</DialogTitle>
					<DialogDescription>
						Create a new markdown rule file. The name becomes{' '}
						<span className="font-mono">name.md</span> on disk.
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-2">
					<Label htmlFor="rule-name">Name</Label>
					<Input
						id="rule-name"
						value={name}
						disabled={creating}
						onChange={(e) => {
							setName(e.target.value)
							setValidationError(null)
						}}
						placeholder="postgres"
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
