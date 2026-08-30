import { useState, useEffect, useId } from 'react'
import { Plus, Trash2 } from 'lucide-react'
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
	DialogDescription,
	DialogFooter,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { IconButton } from '@/components/ui/icon-button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import type { MCPServer, MCPServerEntry } from '../api/mcp-config'

const NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/

const TRANSPORT_OPTIONS = [
	{ value: '', label: '(default)' },
	{ value: 'http', label: 'http' },
	{ value: 'sse', label: 'sse' },
	{ value: 'stdio', label: 'stdio' },
] as const

interface KeyValueRow {
	id: string
	key: string
	value: string
}

interface ServerDraft {
	name: string
	url: string
	transport: string
	command: string
	description: string
	args: string[]
	headers: KeyValueRow[]
	env: KeyValueRow[]
}

interface MCPServerDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onSave: (server: MCPServer, originalName?: string) => void
	initialServer?: MCPServer | null
	existingNames?: string[]
	saving?: boolean
}

const selectClasses = cn(
	'flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
	'disabled:cursor-not-allowed disabled:opacity-50',
)

const textareaClasses = cn(
	'flex w-full resize-y rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm',
	'placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
)

function newRow(): KeyValueRow {
	return { id: crypto.randomUUID(), key: '', value: '' }
}

function mapToRows(record?: Record<string, string>): KeyValueRow[] {
	if (!record || Object.keys(record).length === 0) {
		return [newRow()]
	}
	return Object.entries(record).map(([key, value]) => ({
		id: crypto.randomUUID(),
		key,
		value,
	}))
}

function rowsToRecord(rows: KeyValueRow[]): Record<string, string> | undefined {
	const out: Record<string, string> = {}
	for (const row of rows) {
		const key = row.key.trim()
		if (!key) continue
		out[key] = row.value
	}
	return Object.keys(out).length > 0 ? out : undefined
}

function serverToDraft(server: MCPServer): ServerDraft {
	return {
		name: server.name,
		url: server.url ?? '',
		transport: server.transport ?? '',
		command: server.command ?? '',
		description: server.description ?? '',
		args: server.args?.length ? [...server.args] : [''],
		headers: mapToRows(server.headers),
		env: mapToRows(server.env),
	}
}

const emptyDraft: ServerDraft = {
	name: '',
	url: '',
	transport: '',
	command: '',
	description: '',
	args: [''],
	headers: [newRow()],
	env: [newRow()],
}

function draftToServer(draft: ServerDraft): MCPServer {
	const entry: MCPServerEntry = {}
	const url = draft.url.trim()
	const command = draft.command.trim()
	const transport = draft.transport.trim()
	const description = draft.description.trim()

	if (url) entry.url = url
	if (transport) entry.transport = transport
	if (command) entry.command = command
	if (description) entry.description = description

	const headers = rowsToRecord(draft.headers)
	if (headers) entry.headers = headers

	const env = rowsToRecord(draft.env)
	if (env) entry.env = env

	const args = draft.args.map((a) => a.trim()).filter(Boolean)
	if (args.length > 0) entry.args = args

	return { name: draft.name.trim(), ...entry }
}

export function MCPServerDialog({
	open,
	onOpenChange,
	onSave,
	initialServer,
	existingNames = [],
	saving = false,
}: MCPServerDialogProps) {
	const formId = useId()
	const isEditing = !!initialServer
	const [draft, setDraft] = useState<ServerDraft>(emptyDraft)
	const [validationError, setValidationError] = useState<string | null>(null)

	useEffect(() => {
		if (open) {
			setDraft(initialServer ? serverToDraft(initialServer) : emptyDraft)
			setValidationError(null)
		}
	}, [open, initialServer])

	function updateDraft(patch: Partial<ServerDraft>) {
		setDraft((prev) => ({ ...prev, ...patch }))
		setValidationError(null)
	}

	function updateKeyValueRows(
		field: 'headers' | 'env',
		id: string,
		patch: Partial<KeyValueRow>,
	) {
		setDraft((prev) => ({
			...prev,
			[field]: prev[field].map((row) =>
				row.id === id ? { ...row, ...patch } : row,
			),
		}))
		setValidationError(null)
	}

	function addKeyValueRow(field: 'headers' | 'env') {
		setDraft((prev) => ({
			...prev,
			[field]: [...prev[field], newRow()],
		}))
	}

	function removeKeyValueRow(field: 'headers' | 'env', id: string) {
		setDraft((prev) => {
			const rows = prev[field].filter((row) => row.id !== id)
			return {
				...prev,
				[field]: rows.length > 0 ? rows : [newRow()],
			}
		})
	}

	function updateArg(index: number, value: string) {
		setDraft((prev) => {
			const args = [...prev.args]
			args[index] = value
			return { ...prev, args }
		})
		setValidationError(null)
	}

	function addArg() {
		setDraft((prev) => ({ ...prev, args: [...prev.args, ''] }))
	}

	function removeArg(index: number) {
		setDraft((prev) => {
			const args = prev.args.filter((_, i) => i !== index)
			return { ...prev, args: args.length > 0 ? args : [''] }
		})
	}

	function handleSave() {
		const trimmedName = draft.name.trim()
		if (!trimmedName) {
			setValidationError('Name is required')
			return
		}
		if (!NAME_PATTERN.test(trimmedName)) {
			setValidationError(
				'Use letters, numbers, hyphens, and underscores; start with a letter or number',
			)
			return
		}
		const taken = existingNames.filter(
			(n) => !isEditing || n !== initialServer?.name,
		)
		if (taken.includes(trimmedName)) {
			setValidationError('A server with this name already exists')
			return
		}
		if (!draft.url.trim() && !draft.command.trim()) {
			setValidationError('Provide either a URL or a command')
			return
		}

		const server = draftToServer(draft)
		onSave(server, isEditing ? initialServer?.name : undefined)
	}

	function renderKeyValueSection(
		field: 'headers' | 'env',
		label: string,
		description: string,
	) {
		return (
			<div className="space-y-2">
				<div>
					<Label>{label}</Label>
					<p className="text-xs text-muted-foreground">{description}</p>
				</div>
				<div className="space-y-2">
					{draft[field].map((row) => (
						<div key={row.id} className="flex gap-2">
							<Input
								value={row.key}
								onChange={(e) =>
									updateKeyValueRows(field, row.id, {
										key: e.target.value,
									})
								}
								placeholder="Key"
								disabled={saving}
								className="font-mono"
							/>
							<Input
								value={row.value}
								onChange={(e) =>
									updateKeyValueRows(field, row.id, {
										value: e.target.value,
									})
								}
								placeholder="Value"
								disabled={saving}
							/>
							<IconButton
								type="button"
								variant="outline"
								onClick={() => removeKeyValueRow(field, row.id)}
								disabled={saving}
								tooltip={`Remove ${label.toLowerCase()} row`}
							>
								<Trash2 className="h-4 w-4" />
							</IconButton>
						</div>
					))}
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => addKeyValueRow(field)}
						disabled={saving}
					>
						<Plus className="mr-2 h-4 w-4" />
						Add row
					</Button>
				</div>
			</div>
		)
	}

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!saving) onOpenChange(next)
			}}
		>
			<DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
				<DialogHeader>
					<DialogTitle>
						{isEditing ? 'Edit MCP server' : 'Add MCP server'}
					</DialogTitle>
					<DialogDescription>
						Configure a server entry in{' '}
						<span className="font-mono">mcp.json</span>. Use URL for
						remote servers or command for stdio processes. Any
						value may reference a secret from Variables → Secrets
						as <span className="font-mono">{'${SECRET_NAME}'}</span>.
					</DialogDescription>
				</DialogHeader>

				<div className="space-y-4">
					<div className="space-y-2">
						<Label htmlFor={`${formId}-name`}>Name</Label>
						<Input
							id={`${formId}-name`}
							value={draft.name}
							onChange={(e) => updateDraft({ name: e.target.value })}
							placeholder="github"
							disabled={saving}
							className="font-mono"
						/>
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${formId}-url`}>URL</Label>
						<Input
							id={`${formId}-url`}
							value={draft.url}
							onChange={(e) => updateDraft({ url: e.target.value })}
							placeholder="https://example.com/mcp"
							disabled={saving}
						/>
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${formId}-transport`}>Transport</Label>
						<select
							id={`${formId}-transport`}
							value={draft.transport}
							onChange={(e) =>
								updateDraft({ transport: e.target.value })
							}
							disabled={saving}
							className={selectClasses}
						>
							{TRANSPORT_OPTIONS.map((opt) => (
								<option key={opt.value || 'default'} value={opt.value}>
									{opt.label}
								</option>
							))}
						</select>
					</div>

					{renderKeyValueSection(
						'headers',
						'Headers',
						'Optional HTTP headers for remote servers. Reference a secret from Variables → Secrets with ${SECRET_NAME}, e.g. Bearer ${MCP_GITHUB_TOKEN}.',
					)}

					<div className="space-y-2">
						<Label htmlFor={`${formId}-command`}>Command</Label>
						<Input
							id={`${formId}-command`}
							value={draft.command}
							onChange={(e) =>
								updateDraft({ command: e.target.value })
							}
							placeholder="npx"
							disabled={saving}
							className="font-mono"
						/>
					</div>

					<div className="space-y-2">
						<Label>Args</Label>
						<div className="space-y-2">
							{draft.args.map((arg, index) => (
								<div key={index} className="flex gap-2">
									<Input
										value={arg}
										onChange={(e) => updateArg(index, e.target.value)}
										placeholder="-y @upstash/context7-mcp"
										disabled={saving}
										className="font-mono"
									/>
									<IconButton
										type="button"
										variant="outline"
										onClick={() => removeArg(index)}
										disabled={saving}
										tooltip="Remove argument"
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								</div>
							))}
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={addArg}
								disabled={saving}
							>
								<Plus className="mr-2 h-4 w-4" />
								Add argument
							</Button>
						</div>
					</div>

					{renderKeyValueSection(
						'env',
						'Environment',
						'Optional environment variables for stdio commands.',
					)}

					<div className="space-y-2">
						<Label htmlFor={`${formId}-description`}>Description</Label>
						<textarea
							id={`${formId}-description`}
							value={draft.description}
							onChange={(e) =>
								updateDraft({ description: e.target.value })
							}
							rows={2}
							disabled={saving}
							className={textareaClasses}
							placeholder="Optional notes about this server"
						/>
					</div>

					{validationError && (
						<p className="text-sm text-destructive">{validationError}</p>
					)}
				</div>

				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						onClick={() => onOpenChange(false)}
						disabled={saving}
					>
						Cancel
					</Button>
					<Button type="button" onClick={handleSave} disabled={saving}>
						{saving ? 'Saving...' : isEditing ? 'Save' : 'Create'}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
