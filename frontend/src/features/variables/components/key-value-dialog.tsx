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
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import {
	addsAllowedHost,
	parseAllowedHosts,
} from '@/features/secrets/lib/allowed-hosts'
import type { VariableKind } from '../api/variables'

export interface KeyValueInput {
	scope?: string
	scopeName?: string
	name: string
	description: string
	value: string
	kind?: VariableKind
	allowedHosts?: string[]
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

export function KeyValueDialog({
	open,
	onOpenChange,
	onSave,
	kind,
	initial,
}: KeyValueDialogProps) {
	const [draft, setDraft] = useState<KeyValueInput>(emptyInput)
	const [hostsText, setHostsText] = useState('')
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
			setHostsText((initial?.allowedHosts ?? []).join('\n'))
		}
	}, [open, initial, kind, isEditing])

	const isSecret = kind === 'secret'
	const allowedHosts = parseAllowedHosts(hostsText)
	// Adding a host widens where mcp.json may send the value, so the backend
	// takes it only together with the value; removing one needs nothing.
	const hostsNeedValue =
		isSecret &&
		isEditing &&
		(draft.value?.trim() ?? '') === '' &&
		addsAllowedHost(initial?.allowedHosts ?? [], allowedHosts)

	function handleSave() {
		const trimmedScope = draft.scope?.trim() || 'global'
		onSave({
			...draft,
			...(isSecret ? { allowedHosts } : {}),
			scope: trimmedScope,
			scopeName:
				trimmedScope === 'global'
					? ''
					: draft.scopeName?.trim() || '',
		})
	}

	const scope = draft.scope?.trim() || 'global'
	const needsScopeName = scope !== 'global'
	// ${NAME} in mcp.json reads the global scope only.
	const showHosts = isSecret && scope === 'global'

	const canSave =
		draft.name.trim() !== '' &&
		(!needsScopeName || (draft.scopeName?.trim() ?? '') !== '') &&
		(isEditing || kind === 'secret' || isList || (draft.value?.trim() ?? '') !== '') &&
		!hostsNeedValue

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
							<Select
								id={`${kind}-type`}
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
							</Select>
						</div>
					)}
					<div className="space-y-2">
						<Label htmlFor={`${kind}-scope`}>Scope</Label>
						<Select
							id={`${kind}-scope`}
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
						</Select>
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
						<Textarea
							id={`${kind}-value`}
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
					{showHosts && (
						<div className="space-y-2">
							<Label htmlFor="secret-allowed-hosts">Allowed hosts</Label>
							<Textarea
								id="secret-allowed-hosts"
								rows={3}
								value={hostsText}
								placeholder="api.example.com"
								onChange={(e) => setHostsText(e.target.value)}
							/>
							<p className="text-xs text-muted-foreground">
								Hosts an mcp.json server may send this secret to through{' '}
								<code>{'${' + (draft.name.trim() || 'NAME') + '}'}</code>, one per
								line, without scheme or port. Leave empty to keep the secret out of
								mcp.json.
							</p>
							{hostsNeedValue && (
								<p className="text-xs text-destructive">
									Enter the value again to add a host.
								</p>
							)}
						</div>
					)}
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
