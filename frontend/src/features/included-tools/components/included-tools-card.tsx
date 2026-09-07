import { useCallback, useEffect, useId, useMemo, useState } from 'react'
import {
	ChevronLeft,
	ChevronRight,
	ChevronsLeft,
	ChevronsRight,
	Save,
	Server,
	Sparkles,
	X,
} from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { Select } from '@/components/ui/select'
import { extractErrorMessage } from '@/lib/api-client'
import { listModes } from '@/features/modes/api/modes'
import { useMode } from '@/features/modes/mode-context'
import { useDialog } from '@/features/dialogs/dialog-context'
import {
	createDialog,
	openDialogActivity,
} from '@/features/dialogs/api/dialogs'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import {
	getIncludedTools,
	setIncludedTools,
	listCatalogTools,
	type CatalogTool,
} from '../api/included-tools'
import {
	formatServerLabel,
	groupByServer,
	matchesFilter,
	type ToolEntry,
	unknownServer,
} from '../lib/group-tools'
import { summarizeToolDescription } from '../lib/tool-description'
import { buildDistributeToolsRequest } from '../lib/build-distribute-request'

function ToolName({
	name,
	description,
}: {
	name: string
	description: string
}) {
	const summary = summarizeToolDescription(description)

	if (!summary) {
		return (
			<span className="min-w-0 flex-1 truncate font-mono font-medium">
				{name}
			</span>
		)
	}

	return (
		<Tooltip>
			<TooltipTrigger asChild>
				<span className="min-w-0 flex-1 truncate font-mono font-medium">
					{name}
				</span>
			</TooltipTrigger>
			<TooltipContent
				side="top"
				className="max-w-xs whitespace-normal text-left leading-relaxed"
			>
				{summary}
			</TooltipContent>
		</Tooltip>
	)
}

function ToolListPanel({
	title,
	helpText,
	tools,
	filter,
	onMove,
	onMoveMany,
	moveLabel,
	moveAllLabel,
	direction,
}: {
	title: string
	helpText: string
	tools: ToolEntry[]
	filter: string
	onMove: (name: string) => void
	onMoveMany: (names: string[]) => void
	moveLabel: string
	moveAllLabel: string
	direction: 'left' | 'right'
}) {
	const grouped = useMemo(() => groupByServer(tools), [tools])
	const filtered = useMemo(
		() => tools.filter((t) => matchesFilter(t, filter)),
		[tools, filter],
	)
	const filteredNames = useMemo(
		() => new Set(filtered.map((t) => t.name)),
		[filtered],
	)
	const MoveAllIcon = direction === 'right' ? ChevronsRight : ChevronsLeft
	const MoveServerIcon = direction === 'right' ? ChevronRight : ChevronLeft

	const handleMoveAll = () => {
		onMoveMany(filtered.map((t) => t.name))
	}

	const handleMoveServer = (serverTools: ToolEntry[]) => {
		const visible = serverTools.filter((t) => filteredNames.has(t.name))
		onMoveMany(visible.map((t) => t.name))
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col rounded-lg border">
			<div className="border-b px-3 py-2">
				<div className="flex items-start justify-between gap-2">
					<div className="min-w-0">
						<h3 className="text-sm font-medium">{title}</h3>
						<p className="text-xs text-muted-foreground">{helpText}</p>
						<p className="mt-1 text-xs text-muted-foreground">
							{filtered.length} tool{filtered.length === 1 ? '' : 's'}
						</p>
					</div>
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={handleMoveAll}
						disabled={filtered.length === 0}
						aria-label={`${moveAllLabel} ${title.toLowerCase()} tools`}
						className="shrink-0"
					>
						<MoveAllIcon className="h-4 w-4" />
						{moveAllLabel}
					</Button>
				</div>
			</div>
			<div className="min-h-0 flex-1 overflow-y-auto p-2">
				{filtered.length === 0 ? (
					<p className="px-2 py-4 text-sm text-muted-foreground">
						No tools in this list.
					</p>
				) : (
					<div className="space-y-3">
						{[...grouped.entries()].map(([server, serverTools]) => {
							const visible = serverTools.filter((t) =>
								filteredNames.has(t.name),
							)
							if (visible.length === 0) {
								return null
							}
							const serverLabel = formatServerLabel(server)
							return (
								<section
									key={server || 'unknown'}
									className="overflow-hidden rounded-md border border-border/60 bg-muted/20"
								>
									<div
										className={cn(
											'sticky top-0 z-10 flex items-center gap-2',
											'border-b border-border/50 bg-card/95 px-2 py-1.5',
											'backdrop-blur supports-[backdrop-filter]:bg-card/80',
										)}
									>
										<Server className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
										<span className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
											{serverLabel}
										</span>
										<Badge variant="secondary" className="shrink-0">
											{visible.length}
										</Badge>
										<Button
											type="button"
											variant="ghost"
											size="icon-sm"
											onClick={() => handleMoveServer(serverTools)}
											aria-label={`${moveAllLabel} all ${serverLabel} tools`}
											className="shrink-0"
										>
											<MoveServerIcon className="h-4 w-4" />
										</Button>
									</div>
									<ul className="space-y-1 p-2">
										{visible.map((tool) => (
											<li key={tool.name}>
												<button
													type="button"
													aria-label={`${moveLabel} ${tool.name}`}
													onClick={() => onMove(tool.name)}
													className={cn(
														'flex w-full cursor-pointer items-center justify-between',
														'gap-2 rounded-md border border-border/60 px-2 py-1.5',
														'text-left text-sm transition-colors focus-ring',
														'duration-[var(--dur-fast)] hover:border-ring/40',
														'hover:bg-foreground/[0.05]',
													)}
												>
													{direction === 'left' && (
														<ChevronLeft className="h-4 w-4 shrink-0 text-muted-foreground" />
													)}
													<ToolName
														name={tool.name}
														description={tool.description}
													/>
													{direction === 'right' && (
														<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
													)}
												</button>
											</li>
										))}
									</ul>
								</section>
							)
						})}
					</div>
				)}
			</div>
		</div>
	)
}

export function IncludedToolsCard() {
	const modeSelectId = useId()
	const filterId = useId()
	const [modes, setModes] = useState<string[]>([])
	const [selectedMode, setSelectedMode] = useState('')
	const [catalog, setCatalog] = useState<CatalogTool[]>([])
	const [included, setIncluded] = useState<Set<string>>(new Set())
	const [savedIncluded, setSavedIncluded] = useState<Set<string>>(new Set())
	const [filter, setFilter] = useState('')
	const [modesLoading, setModesLoading] = useState(true)
	const [modeLoading, setModeLoading] = useState(false)
	const [catalogLoading, setCatalogLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const [distributing, setDistributing] = useState(false)
	const [distributeDialogId, setDistributeDialogId] = useState<string | null>(
		null,
	)
	const [error, setError] = useState<string | null>(null)

	const { selectMode } = useMode()
	const { setActiveDialogId, bumpDialogsVersion, enqueuePendingMessage } =
		useDialog()

	const catalogEntries = useMemo<ToolEntry[]>(
		() =>
			catalog.map((t) => ({
				name: t.name,
				server: t.server,
				description: t.description,
			})),
		[catalog],
	)

	const catalogNames = useMemo(
		() => new Set(catalogEntries.map((t) => t.name)),
		[catalogEntries],
	)

	const allToolEntries = useMemo(() => {
		const byName = new Map<string, ToolEntry>()
		for (const t of catalogEntries) {
			byName.set(t.name, t)
		}
		for (const name of included) {
			if (!byName.has(name)) {
				byName.set(name, {
					name,
					server: unknownServer,
					description: '',
				})
			}
		}
		return [...byName.values()].sort((a, b) =>
			a.name.localeCompare(b.name),
		)
	}, [catalogEntries, included])

	const notIncludedTools = useMemo(
		() => allToolEntries.filter((t) => !included.has(t.name)),
		[allToolEntries, included],
	)

	const includedTools = useMemo(
		() => allToolEntries.filter((t) => included.has(t.name)),
		[allToolEntries, included],
	)

	const dirty = useMemo(() => {
		if (included.size !== savedIncluded.size) {
			return true
		}
		for (const name of included) {
			if (!savedIncluded.has(name)) {
				return true
			}
		}
		return false
	}, [included, savedIncluded])

	const loadModeTools = useCallback(async (mode: string) => {
		const data = await getIncludedTools(mode)
		const includedSet = new Set(data.includedTools ?? [])
		setIncluded(includedSet)
		setSavedIncluded(new Set(includedSet))
	}, [])

	useEffect(() => {
		let cancelled = false

		async function load() {
			setModesLoading(true)
			setError(null)
			try {
				const modeList = await listModes()
				if (cancelled) {
					return
				}
				setModes(modeList)
				if (modeList.length > 0) {
					setSelectedMode(modeList[0] ?? '')
				}
			} catch (err) {
				if (!cancelled) {
					setError(extractErrorMessage(err))
				}
			} finally {
				if (!cancelled) {
					setModesLoading(false)
				}
			}
		}

		void load()
		return () => {
			cancelled = true
		}
	}, [])

	useEffect(() => {
		let cancelled = false

		async function load() {
			setCatalogLoading(true)
			setError(null)
			try {
				const tools = await listCatalogTools()
				if (!cancelled) {
					setCatalog(tools)
				}
			} catch (err) {
				if (!cancelled) {
					setError(extractErrorMessage(err))
				}
			} finally {
				if (!cancelled) {
					setCatalogLoading(false)
				}
			}
		}

		void load()
		return () => {
			cancelled = true
		}
	}, [])

	useEffect(() => {
		if (!selectedMode) {
			return
		}

		let cancelled = false

		async function load() {
			setModeLoading(true)
			setError(null)
			try {
				await loadModeTools(selectedMode)
			} catch (err) {
				if (!cancelled) {
					setError(extractErrorMessage(err))
				}
			} finally {
				if (!cancelled) {
					setModeLoading(false)
				}
			}
		}

		void load()
		return () => {
			cancelled = true
		}
	}, [selectedMode, loadModeTools])

	const includeTool = useCallback((name: string) => {
		setIncluded((prev) => {
			const next = new Set(prev)
			next.add(name)
			return next
		})
	}, [])

	const excludeTool = useCallback((name: string) => {
		setIncluded((prev) => {
			const next = new Set(prev)
			next.delete(name)
			return next
		})
	}, [])

	const includeTools = useCallback((names: string[]) => {
		if (names.length === 0) {
			return
		}
		setIncluded((prev) => {
			const next = new Set(prev)
			for (const name of names) {
				next.add(name)
			}
			return next
		})
	}, [])

	const excludeTools = useCallback((names: string[]) => {
		if (names.length === 0) {
			return
		}
		setIncluded((prev) => {
			const next = new Set(prev)
			for (const name of names) {
				next.delete(name)
			}
			return next
		})
	}, [])

	const handleSave = useCallback(async () => {
		if (!selectedMode) {
			return
		}
		setSaving(true)
		setError(null)
		try {
			const data = await setIncludedTools(
				selectedMode,
				Array.from(included).sort(),
			)
			const includedSet = new Set(data.includedTools ?? [])
			setIncluded(includedSet)
			setSavedIncluded(new Set(includedSet))
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}, [selectedMode, included])

	// The distribution runs in a discuss dialog; reload the selected mode's list
	// as the agent writes so the panels follow it. Skipped while the card is
	// dirty - reloading would throw away shuttle moves the operator has not
	// saved, and the button is disabled in that state anyway.
	useEffect(() => {
		if (!distributeDialogId || !selectedMode || dirty) {
			return
		}

		const close = openDialogActivity(distributeDialogId, (ev) => {
			if (ev.kind === 'tools_end' || ev.kind === 'turn_end') {
				void loadModeTools(selectedMode).catch((err) => {
					setError(extractErrorMessage(err))
				})
			}
		})

		return close
	}, [distributeDialogId, selectedMode, dirty, loadModeTools])

	const handleDistribute = useCallback(async () => {
		if (catalog.length === 0) {
			return
		}

		setDistributing(true)
		setError(null)
		try {
			// Only discuss carries the skills list and the update_included_tools
			// tool, and the system prompt is frozen on a dialog's first message -
			// so the mode has to be settled before anything is sent.
			selectMode('discuss')
			const dialog = await createDialog({
				mode: 'discuss',
				title: 'Included tools: distribute',
			})
			// The chat panel picks this up once it has loaded the (empty)
			// transcript for the newly active dialog and sends it for us.
			enqueuePendingMessage({
				dialogId: dialog.id,
				text: buildDistributeToolsRequest(modes, catalog),
			})
			setActiveDialogId(dialog.id)
			setDistributeDialogId(dialog.id)
			bumpDialogsVersion()
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setDistributing(false)
		}
	}, [
		catalog,
		modes,
		selectMode,
		enqueuePendingMessage,
		setActiveDialogId,
		bumpDialogsVersion,
	])

	const loading = modesLoading || modeLoading || catalogLoading

	return (
		<Card>
			<CardHeader className="pb-3">
				<div className="space-y-3">
					<CardTitle className="text-base">Included tools</CardTitle>
					<div className="flex min-w-0 items-center gap-2">
						<label
							htmlFor={modeSelectId}
							className="text-sm text-muted-foreground"
						>
							Mode
						</label>
						<Select
							id={modeSelectId}
							wrapperClassName="max-w-xs"
							value={selectedMode}
							onChange={(e) => setSelectedMode(e.target.value)}
							disabled={
								modesLoading || saving || modes.length === 0
							}
						>
							{modes.map((mode) => (
								<option key={mode} value={mode}>
									{mode}
								</option>
							))}
						</Select>
					</div>
					<p className="text-sm text-muted-foreground">
						Distribute opens a discuss chat that narrows{' '}
						<strong>every</strong> mode's list, not just the one
						selected above. It only removes tools - a tool taken out
						stays reachable through search, and putting one back is a
						move here. Save or discard your changes first.
					</p>
					<div className="flex justify-end gap-2">
						<Button
							size="sm"
							variant="outline"
							onClick={() => void handleDistribute()}
							disabled={
								distributing ||
								loading ||
								saving ||
								dirty ||
								catalog.length === 0
							}
						>
							<Sparkles className="mr-2 h-4 w-4" />
							{distributing
								? 'Opening chat...'
								: 'Distribute with AI'}
						</Button>
						<Button
							size="sm"
							onClick={() => void handleSave()}
							disabled={
								!dirty ||
								saving ||
								modeLoading ||
								!selectedMode
							}
						>
							<Save className="mr-2 h-4 w-4" />
							Save
						</Button>
					</div>
				</div>
			</CardHeader>
			<CardContent className="space-y-4">
				{error && (
					<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
						{error}
					</div>
				)}

				<div className="space-y-2">
					<label
						htmlFor={filterId}
						className="text-sm text-muted-foreground"
					>
						Filter tools
					</label>
					<div className="relative">
						<Input
							id={filterId}
							value={filter}
							onChange={(e) => setFilter(e.target.value)}
							placeholder="Search by name, server, or description"
							disabled={loading || saving}
							className={filter ? 'pr-8' : undefined}
						/>
						{filter ? (
							<button
								type="button"
								aria-label="Clear filter"
								onClick={() => setFilter('')}
								disabled={loading || saving}
								className={cn(
									'absolute right-1 top-1/2 flex size-6 -translate-y-1/2',
									'cursor-pointer items-center justify-center rounded-md',
									'text-muted-foreground transition-colors focus-ring',
									'duration-[var(--dur-fast)] hover:bg-foreground/[0.07]',
									'hover:text-foreground disabled:pointer-events-none',
									'disabled:opacity-50',
								)}
							>
								<X className="h-4 w-4" />
							</button>
						) : null}
					</div>
				</div>

				{loading ? (
					<p className="text-sm text-muted-foreground">
						Loading tool catalog...
					</p>
				) : catalogNames.size === 0 ? (
					<p className="text-sm text-muted-foreground">
						No tools in the catalog yet. Configure MCP servers on
						the MCP Servers tab and wait for indexing to finish.
					</p>
				) : (
					<div className="flex min-h-[320px] flex-col gap-3 lg:flex-row">
						<ToolListPanel
							title="Not included"
							helpText="Discoverable via tool_search in this mode."
							tools={notIncludedTools}
							filter={filter}
							onMove={includeTool}
							onMoveMany={includeTools}
							moveLabel="Include"
							moveAllLabel="Include all"
							direction="right"
						/>
						<ToolListPanel
							title="Included"
							helpText="Connected immediately in this mode."
							tools={includedTools}
							filter={filter}
							onMove={excludeTool}
							onMoveMany={excludeTools}
							moveLabel="Exclude"
							moveAllLabel="Exclude all"
							direction="left"
						/>
					</div>
				)}
			</CardContent>
		</Card>
	)
}
