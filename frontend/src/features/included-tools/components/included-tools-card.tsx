import { useCallback, useEffect, useId, useMemo, useState } from 'react'
import { ChevronLeft, ChevronRight, Save, Server, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { extractErrorMessage } from '@/lib/api-client'
import { listModes } from '@/features/modes/api/modes'
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
import { summarizeToolDescription } from '../lib/tool-description'

const selectClasses = cn(
	'flex h-9 w-full max-w-xs rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
)

type ToolEntry = {
	name: string
	server: string
	description: string
}

const unknownServer = ''

function groupByServer(tools: ToolEntry[]): Map<string, ToolEntry[]> {
	const groups = new Map<string, ToolEntry[]>()
	for (const tool of tools) {
		const list = groups.get(tool.server) ?? []
		list.push(tool)
		groups.set(tool.server, list)
	}
	for (const [, list] of groups) {
		list.sort((a, b) => a.name.localeCompare(b.name))
	}
	return new Map([...groups.entries()].sort(([a], [b]) => {
		if (a === unknownServer) {
			return 1
		}
		if (b === unknownServer) {
			return -1
		}
		return a.localeCompare(b)
	}))
}

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

function matchesFilter(tool: ToolEntry, filter: string): boolean {
	if (!filter) {
		return true
	}
	const q = filter.toLowerCase()
	return (
		tool.name.toLowerCase().includes(q) ||
		tool.server.toLowerCase().includes(q) ||
		tool.description.toLowerCase().includes(q)
	)
}

function ToolListPanel({
	title,
	helpText,
	tools,
	filter,
	onMove,
	moveLabel,
	direction,
}: {
	title: string
	helpText: string
	tools: ToolEntry[]
	filter: string
	onMove: (name: string) => void
	moveLabel: string
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

	return (
		<div className="flex min-h-0 flex-1 flex-col rounded-lg border">
			<div className="border-b px-3 py-2">
				<h3 className="text-sm font-medium">{title}</h3>
				<p className="text-xs text-muted-foreground">{helpText}</p>
				<p className="mt-1 text-xs text-muted-foreground">
					{filtered.length} tool{filtered.length === 1 ? '' : 's'}
				</p>
			</div>
			<div className="min-h-0 flex-1 overflow-y-auto p-2">
				{filtered.length === 0 ? (
					<p className="px-2 py-4 text-sm text-muted-foreground">
						No tools in this list.
					</p>
				) : (
					[...grouped.entries()].map(([server, serverTools]) => {
						const visible = serverTools.filter((t) =>
							filteredNames.has(t.name),
						)
						if (visible.length === 0) {
							return null
						}
						return (
							<div key={server || 'unknown'} className="mb-3 last:mb-0">
								{server && (
									<div className="mb-1 flex items-center gap-1.5 px-1 text-xs font-medium text-muted-foreground">
										<Server className="h-3 w-3" />
										{server}
									</div>
								)}
								<ul className="space-y-1">
									{visible.map((tool) => (
										<li key={tool.name}>
											<button
												type="button"
												aria-label={`${moveLabel} ${tool.name}`}
												onClick={() => onMove(tool.name)}
												className="flex w-full items-center justify-between gap-2 rounded-md border px-2 py-1.5 text-left text-sm hover:bg-muted/50"
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
							</div>
						)
					})
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
	const [error, setError] = useState<string | null>(null)

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
						<select
							id={modeSelectId}
							className={selectClasses}
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
						</select>
					</div>
					<div className="flex justify-end">
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
					<div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
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
								className="absolute right-1 top-1/2 -translate-y-1/2 rounded-sm p-1 text-muted-foreground hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
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
							moveLabel="Include"
							direction="right"
						/>
						<ToolListPanel
							title="Included"
							helpText="Connected immediately in this mode."
							tools={includedTools}
							filter={filter}
							onMove={excludeTool}
							moveLabel="Exclude"
							direction="left"
						/>
					</div>
				)}
			</CardContent>
		</Card>
	)
}
