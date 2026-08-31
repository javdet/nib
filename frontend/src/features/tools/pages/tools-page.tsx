import { useState, useEffect, useCallback } from 'react'
import {
	Plus,
	Pencil,
	Server,
	Trash2,
	Wrench,
	ChevronDown,
	ChevronRight,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import { extractErrorMessage } from '@/lib/api-client'
import { MCPServerDialog } from '@/features/mcp-config/components/mcp-server-dialog'
import { MCPServerToolsDialog } from '@/features/mcp-config/components/mcp-server-tools-dialog'
import { MCPRawEditor } from '@/features/mcp-config/components/mcp-raw-editor'
import { ExecutorConfigCard } from '@/features/executor/components/executor-config-card'
import { IncludedToolsCard } from '@/features/included-tools/components/included-tools-card'
import { ToolCategoriesCard } from '@/features/tool-categories/components/tool-categories-card'
import {
	listMCPServers,
	createMCPServer,
	updateMCPServer,
	deleteMCPServer,
	getMCPConfigRaw,
	updateMCPConfigRaw,
	type MCPServer,
} from '@/features/mcp-config/api/mcp-config'

type ViewMode = 'servers' | 'raw'

export function ToolsPage() {
	const [view, setView] = useState<ViewMode>('servers')
	const [servers, setServers] = useState<MCPServer[]>([])
	const [listLoading, setListLoading] = useState(true)
	const [dialogOpen, setDialogOpen] = useState(false)
	const [editingServer, setEditingServer] = useState<MCPServer | null>(null)
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)

	const [rawContent, setRawContent] = useState('')
	const [savedRawContent, setSavedRawContent] = useState('')
	const [rawLoading, setRawLoading] = useState(false)
	const [rawSaved, setRawSaved] = useState(false)
	const [rawWarnings, setRawWarnings] = useState<string[]>([])
	const [serversListExpanded, setServersListExpanded] = useState(true)
	const [toolsDialogServer, setToolsDialogServer] =
		useState<MCPServer | null>(null)

	const rawDirty = rawContent !== savedRawContent

	const refreshServers = useCallback(async () => {
		const list = await listMCPServers()
		setServers(list)
		return list
	}, [])

	useEffect(() => {
		refreshServers()
			.catch(() => setServers([]))
			.finally(() => setListLoading(false))
	}, [refreshServers])

	const loadRaw = useCallback(async () => {
		setRawLoading(true)
		setError(null)
		setRawSaved(false)
		try {
			const config = await getMCPConfigRaw()
			setRawContent(config.content)
			setSavedRawContent(config.content)
			setRawWarnings(config.warnings)
		} catch (err) {
			setError(extractErrorMessage(err))
			setRawContent('')
			setSavedRawContent('')
			setRawWarnings([])
		} finally {
			setRawLoading(false)
		}
	}, [])

	// Read on mount, not only on entering the raw view: a server written outside
	// "mcpServers" is missing from the servers list with nothing to explain the
	// gap, and that warning comes back with the raw config.
	useEffect(() => {
		void loadRaw()
	}, [view, loadRaw])

	const switchView = useCallback(
		(next: ViewMode) => {
			if (next === view) return
			if (view === 'raw' && rawDirty) {
				const leave = window.confirm(
					'You have unsaved changes in mcp.json. Discard them?',
				)
				if (!leave) return
			}
			setView(next)
			setError(null)
		},
		[view, rawDirty],
	)

	const handleServerSave = useCallback(
		async (server: MCPServer, originalName?: string) => {
			setError(null)
			setSaving(true)
			try {
				if (originalName) {
					await updateMCPServer(originalName, server)
				} else {
					await createMCPServer(server)
				}
				await refreshServers()
				setDialogOpen(false)
				setEditingServer(null)
			} catch (err) {
				setError(extractErrorMessage(err))
			} finally {
				setSaving(false)
			}
		},
		[refreshServers],
	)

	const handleServerDelete = useCallback(
		async (name: string) => {
			const confirmed = window.confirm(
				`Remove MCP server "${name}" from mcp.json?`,
			)
			if (!confirmed) return

			setError(null)
			setSaving(true)
			try {
				await deleteMCPServer(name)
				setServers((prev) => prev.filter((s) => s.name !== name))
			} catch (err) {
				setError(extractErrorMessage(err))
			} finally {
				setSaving(false)
			}
		},
		[],
	)

	const handleRawSave = useCallback(async () => {
		setError(null)
		setSaving(true)
		try {
			const warnings = await updateMCPConfigRaw(rawContent)
			setSavedRawContent(rawContent)
			setRawSaved(true)
			setRawWarnings(warnings)
			await refreshServers()
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}, [rawContent, refreshServers])

	const serverNames = servers.map((s) => s.name)

	return (
		<div className="space-y-6">
			<div>
				<h2 className="text-2xl font-bold tracking-tight">Tools</h2>
				<p className="text-sm text-muted-foreground">
					Manage MCP servers in{' '}
					<span className="font-mono">mcp.json</span> for toolchain and
					assistants, and configure which tools are included per mode.
				</p>
			</div>

			{error && (
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{error}
				</div>
			)}

			{!rawDirty && rawWarnings.length > 0 && (
				<ul className="space-y-1 rounded-md border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-950 dark:text-amber-100">
					{rawWarnings.map((warning) => (
						<li key={warning}>{warning}</li>
					))}
				</ul>
			)}

			<Tabs defaultValue="servers">
				<TabsList>
					<TabsTrigger value="servers">MCP Servers</TabsTrigger>
					<TabsTrigger value="categories">Categories</TabsTrigger>
					<TabsTrigger value="included">Included tools</TabsTrigger>
					<TabsTrigger value="executor">Executor</TabsTrigger>
				</TabsList>

				<TabsContent value="servers">
				<Card>
					<CardHeader className="pb-3">
						<div className="flex items-center justify-between gap-2">
							<div
								role="button"
								tabIndex={0}
								aria-expanded={serversListExpanded}
								className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
								onClick={() =>
									setServersListExpanded((prev) => !prev)
								}
								onKeyDown={(e) => {
									if (e.key === 'Enter' || e.key === ' ') {
										e.preventDefault()
										setServersListExpanded((prev) => !prev)
									}
								}}
							>
								{serversListExpanded ? (
									<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
								) : (
									<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
								)}
								<CardTitle className="text-base">
									MCP Servers
								</CardTitle>
							</div>
							<div className="flex items-center gap-2">
								<div className="inline-flex rounded-md border p-1">
									<Button
										type="button"
										size="sm"
										variant={
											view === 'servers' ? 'secondary' : 'ghost'
										}
										className={cn('rounded-sm')}
										onClick={() => switchView('servers')}
									>
										Servers
									</Button>
									<Button
										type="button"
										size="sm"
										variant={view === 'raw' ? 'secondary' : 'ghost'}
										className={cn('rounded-sm')}
										onClick={() => switchView('raw')}
									>
										Raw JSON
									</Button>
								</div>
								{view === 'servers' && (
									<Button
										size="sm"
										onClick={(e) => {
											e.stopPropagation()
											setEditingServer(null)
											setDialogOpen(true)
										}}
										disabled={listLoading || saving}
									>
										<Plus className="mr-2 h-4 w-4" />
										Add server
									</Button>
								)}
							</div>
						</div>
					</CardHeader>
					{serversListExpanded && (
						<CardContent>
							{view === 'servers' ? (
								listLoading ? (
									<p className="py-4 text-center text-sm text-muted-foreground">
										Loading...
									</p>
								) : servers.length === 0 ? (
									<div className="flex flex-col items-center gap-3 py-6 text-muted-foreground">
										<Server className="h-10 w-10" />
										<p className="text-sm">
											No MCP servers configured yet.
										</p>
										<Button
											onClick={() => {
												setEditingServer(null)
												setDialogOpen(true)
											}}
										>
											Add your first server
										</Button>
									</div>
								) : (
									<div className="space-y-3">
										{servers.map((server) => (
											<div
												key={server.name}
												className="rounded-lg border p-4"
											>
												<div className="flex items-start justify-between gap-4">
													<div className="min-w-0 flex-1 space-y-2">
														<div className="flex flex-wrap items-center gap-2">
															<Server className="h-4 w-4 shrink-0 text-muted-foreground" />
															<span className="font-mono text-sm font-medium">
																{server.name}
															</span>
															{server.transport && (
																<Badge variant="outline">
																	{server.transport}
																</Badge>
															)}
														</div>
														<p className="truncate pl-6 font-mono text-xs text-muted-foreground">
															{server.url ??
																(server.command
																	? [
																			server.command,
																			...(server.args ??
																				[]),
																		].join(' ')
																	: '—')}
														</p>
														{server.description && (
															<p className="line-clamp-2 pl-6 text-xs text-muted-foreground">
																{server.description}
															</p>
														)}
													</div>
													<div className="flex shrink-0 gap-1">
														<Button
															variant="outline"
															size="sm"
															onClick={() =>
																setToolsDialogServer(server)
															}
															disabled={saving || !server.url}
															title={
																server.url
																	? 'Discover tools from this server'
																	: 'Tool discovery requires an HTTP URL; stdio/command servers are not supported in the UI.'
															}
														>
															<Wrench className="mr-1 h-4 w-4" />
															Tools
														</Button>
														<Button
															variant="outline"
															size="sm"
															onClick={() => {
																setEditingServer(server)
																setDialogOpen(true)
															}}
															disabled={saving}
														>
															<Pencil className="mr-1 h-4 w-4" />
															Edit
														</Button>
														<Button
															variant="outline"
															size="sm"
															onClick={() =>
																void handleServerDelete(
																	server.name,
																)
															}
															disabled={saving}
														>
															<Trash2 className="mr-1 h-4 w-4" />
															Delete
														</Button>
													</div>
												</div>
											</div>
										))}
									</div>
								)
							) : (
								<MCPRawEditor
									content={rawContent}
									isDirty={rawDirty}
									loading={rawLoading}
									saving={saving}
									saved={rawSaved}
									onChange={(next) => {
										setRawContent(next)
										setRawSaved(false)
									}}
									onSave={() => void handleRawSave()}
								/>
							)}
						</CardContent>
					)}
				</Card>
				</TabsContent>

				<TabsContent value="categories">
					<ToolCategoriesCard />
				</TabsContent>

				<TabsContent value="included">
					<IncludedToolsCard />
				</TabsContent>

				<TabsContent value="executor">
					<ExecutorConfigCard />
				</TabsContent>
			</Tabs>

			<MCPServerDialog
				open={dialogOpen}
				onOpenChange={(open) => {
					setDialogOpen(open)
					if (!open) setEditingServer(null)
				}}
				onSave={(server, originalName) =>
					void handleServerSave(server, originalName)
				}
				initialServer={editingServer}
				existingNames={serverNames}
				saving={saving}
			/>

			<MCPServerToolsDialog
				open={toolsDialogServer !== null}
				onOpenChange={(open) => {
					if (!open) setToolsDialogServer(null)
				}}
				server={toolsDialogServer}
			/>
		</div>
	)
}
