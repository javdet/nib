import { useState, useEffect } from 'react'
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import type { VariableKind } from '../api/variables'

export interface KeyValueInput {
	scope?: string
	scopeName?: string
	name: string
	description: string
	value: string
	kind?: VariableKind
}

interface KeyValueDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onSave: (input: KeyValueInput) => void
	kind: 'variable' | 'secret'
	initial?: KeyValueInput | null
}

const scopeOptions = [
	{ value: 'global', label: 'Global' },
	{ value: 'project', label: 'Project' },
	{ value: 'environment', label: 'Environment' },
	{ value: 'cloud', label: 'Cloud' },
	{ value: 'location', label: 'Location' },
] as const

function scopeNameLabel(scope: string): string {
	switch (scope) {
		case 'project':
			return 'Project name'
		case 'environment':
			return 'Environment name'
		case 'cloud':
			return 'Cloud name'
		case 'location':
			return 'Location name'
		default:
			return 'Scope name'
	}
}

const emptyInput: KeyValueInput = {
	scope: 'global',
	scopeName: '',
	name: '',
	description: '',
	value: '',
	kind: 'string',
}

const textareaClasses = cn(
	'flex w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm',
	'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
)

const selectClasses = cn(
	'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
	'disabled:cursor-not-allowed disabled:opacity-50',
)

export function KeyValueDialog({
	open,
	onOpenChange,
	onSave,
	kind,
	initial,
}: KeyValueDialogProps) {
	const [draft, setDraft] = useState<KeyValueInput>(emptyInput)
	const isEditing = !!initial
	const label = kind === 'secret' ? 'secret' : 'variable'
	const isList = kind === 'variable' && draft.kind === 'list'

	useEffect(() => {
		if (open) {
			setDraft(
				initial
					? {
							scope: initial.scope,
							scopeName: initial.scopeName ?? '',
							name: initial.name,
							description: initial.description,
							value: kind === 'secret' && isEditing ? '' : initial.value,
							kind: initial.kind ?? 'string',
						}
					: emptyInput,
			)
		}
	}, [open, initial, kind, isEditing])

	function handleSave() {
		const trimmedScope = draft.scope?.trim() || 'global'
		onSave({
			...draft,
			scope: trimmedScope,
			scopeName:
				trimmedScope === 'global'
					? ''
					: draft.scopeName?.trim() || '',
		})
	}

	const scope = draft.scope?.trim() || 'global'
	const needsScopeName = scope !== 'global'

	const canSave =
		draft.name.trim() !== '' &&
		(!needsScopeName || (draft.scopeName?.trim() ?? '') !== '') &&
		(isEditing || kind === 'secret' || isList || (draft.value?.trim() ?? '') !== '')

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-lg">
				<DialogHeader>
					<DialogTitle>
						{isEditing ? `Edit ${label}` : `Create ${label}`}
					</DialogTitle>
				</DialogHeader>

				<div className="space-y-4">
					{kind === 'variable' && (
						<div className="space-y-2">
							<Label htmlFor={`${kind}-type`}>Type</Label>
							<select
								id={`${kind}-type`}
								className={selectClasses}
								value={draft.kind ?? 'string'}
								disabled={isEditing}
								onChange={(e) =>
									setDraft((prev) => ({
										...prev,
										kind: e.target.value as VariableKind,
									}))
								}
							>
								<option value="string">String</option>
								<option value="list">List</option>
							</select>
						</div>
					)}
					<div className="space-y-2">
						<Label htmlFor={`${kind}-scope`}>Scope</Label>
						<select
							id={`${kind}-scope`}
							className={selectClasses}
							value={scope}
							onChange={(e) => {
								const nextScope = e.target.value
								setDraft((prev) => ({
									...prev,
									scope: nextScope,
									scopeName:
										nextScope === 'global' ? '' : prev.scopeName,
								}))
							}}
						>
							{scopeOptions.map((option) => (
								<option key={option.value} value={option.value}>
									{option.label}
								</option>
							))}
						</select>
					</div>
					{needsScopeName && (
						<div className="space-y-2">
							<Label htmlFor={`${kind}-scope-name`}>
								{scopeNameLabel(scope)}
							</Label>
							<Input
								id={`${kind}-scope-name`}
								value={draft.scopeName ?? ''}
								onChange={(e) =>
									setDraft((prev) => ({
										...prev,
										scopeName: e.target.value,
									}))
								}
							/>
						</div>
					)}
					<div className="space-y-2">
						<Label htmlFor={`${kind}-name`}>Name</Label>
						<Input
							id={`${kind}-name`}
							value={draft.name}
							disabled={isEditing}
							onChange={(e) =>
								setDraft((prev) => ({ ...prev, name: e.target.value }))
							}
						/>
					</div>
					<div className="space-y-2">
						<Label htmlFor={`${kind}-description`}>Description</Label>
						<Input
							id={`${kind}-description`}
							value={draft.description}
							onChange={(e) =>
								setDraft((prev) => ({
									...prev,
									description: e.target.value,
								}))
							}
						/>
					</div>
					<div className="space-y-2">
						<Label htmlFor={`${kind}-value`}>Value</Label>
						<textarea
							id={`${kind}-value`}
							className={textareaClasses}
							rows={isList ? 8 : 4}
							value={draft.value}
							placeholder={
								kind === 'secret' && isEditing
									? 'Leave blank to keep current value'
									: isList
										? 'One value per line'
										: undefined
							}
							onChange={(e) =>
								setDraft((prev) => ({ ...prev, value: e.target.value }))
							}
						/>
					</div>
				</div>

				<DialogFooter>
					<Button variant="outline" onClick={() => onOpenChange(false)}>
						Cancel
					</Button>
					<Button onClick={handleSave} disabled={!canSave}>
						Save
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
