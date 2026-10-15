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
import { Textarea } from '@/components/ui/textarea'
import type { MCPServer, MCPServerEntry } from '../api/mcp-config'

const NAME_PATTERN = /^[a-zA-Z0-9][a-zA-Z0-9_-]*$/

interface KeyValueRow {
	id: string
	key: string
	value: string
}

interface ServerDraft {
	name: string
	url: string
	description: string
	headers: KeyValueRow[]
}

interface MCPServerDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onSave: (server: MCPServer, originalName?: string) => void
	initialServer?: MCPServer | null
	existingNames?: string[]
	saving?: boolean
}

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
		description: server.description ?? '',
		headers: mapToRows(server.headers),
	}
}

const emptyDraft: ServerDraft = {
	name: '',
	url: '',
	description: '',
	headers: [newRow()],
}

// Only streamable HTTP is supported, so the form has no command, args, env or
// transport: saving an entry that carried them (from the raw editor) drops them.
function draftToServer(draft: ServerDraft): MCPServer {
	const entry: MCPServerEntry = { url: draft.url.trim() }
	const description = draft.description.trim()
	if (description) entry.description = description

	const headers = rowsToRecord(draft.headers)
	if (headers) entry.headers = headers

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

	function updateHeader(id: string, patch: Partial<KeyValueRow>) {
		setDraft((prev) => ({
			...prev,
			headers: prev.headers.map((row) =>
				row.id === id ? { ...row, ...patch } : row,
			),
		}))
		setValidationError(null)
	}

	function addHeader() {
		setDraft((prev) => ({ ...prev, headers: [...prev.headers, newRow()] }))
	}

	function removeHeader(id: string) {
		setDraft((prev) => {
			const headers = prev.headers.filter((row) => row.id !== id)
			return { ...prev, headers: headers.length > 0 ? headers : [newRow()] }
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
		if (!draft.url.trim()) {
			setValidationError('URL is required')
			return
		}

		const server = draftToServer(draft)
		onSave(server, isEditing ? initialServer?.name : undefined)
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
						Nib connects over streamable HTTP only, so register
						the server&apos;s HTTP endpoint; a stdio server needs an
						HTTP bridge in front of it. Any value may reference a secret from Variables → Secrets
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
						<div>
							<Label>Headers</Label>
							<p className="text-xs text-muted-foreground">
								Optional HTTP headers. Reference a secret from
								Variables → Secrets with{' '}
								<span className="font-mono">{'${SECRET_NAME}'}</span>,
								e.g.{' '}
								<span className="font-mono">
									{'Bearer ${MCP_GITHUB_TOKEN}'}
								</span>
								.
							</p>
						</div>
						<div className="space-y-2">
							{draft.headers.map((row) => (
								<div key={row.id} className="flex gap-2">
									<Input
										value={row.key}
										onChange={(e) =>
											updateHeader(row.id, { key: e.target.value })
										}
										placeholder="Key"
										disabled={saving}
										className="font-mono"
									/>
									<Input
										value={row.value}
										onChange={(e) =>
											updateHeader(row.id, { value: e.target.value })
										}
										placeholder="Value"
										disabled={saving}
									/>
									<IconButton
										type="button"
										variant="outline"
										onClick={() => removeHeader(row.id)}
										disabled={saving}
										tooltip="Remove header row"
									>
										<Trash2 className="h-4 w-4" />
									</IconButton>
								</div>
							))}
							<Button
								type="button"
								variant="outline"
								size="sm"
								onClick={addHeader}
								disabled={saving}
							>
								<Plus className="mr-2 h-4 w-4" />
								Add row
							</Button>
						</div>
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${formId}-description`}>Description</Label>
						<Textarea
							id={`${formId}-description`}
							value={draft.description}
							onChange={(e) =>
								updateDraft({ description: e.target.value })
							}
							rows={2}
							disabled={saving}
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
