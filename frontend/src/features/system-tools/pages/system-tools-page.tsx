import { useState, useEffect, useMemo } from 'react'
import { ChevronDown, ChevronRight, Wrench } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { extractErrorMessage } from '@/lib/api-client'
import {
	listSystemTools,
	type SystemTool,
} from '../api/system-tools'

function formatParameters(parameters: unknown): string {
	try {
		return JSON.stringify(parameters, null, 2)
	} catch {
		return String(parameters)
	}
}

function ToolCard({ tool }: { tool: SystemTool }) {
	const [schemaOpen, setSchemaOpen] = useState(false)

	return (
		<Card>
			<CardHeader className="pb-3">
				<div className="flex flex-wrap items-start justify-between gap-3">
					<div className="min-w-0 space-y-2">
						<CardTitle className="font-mono text-base">
							{tool.name}
						</CardTitle>
						<p className="text-sm text-muted-foreground">
							{tool.description}
						</p>
					</div>
					<div className="flex flex-wrap gap-1.5">
						{tool.always_on ? (
							<Badge variant="default">always</Badge>
						) : tool.modes.length > 0 ? (
							tool.modes.map((mode) => (
								<Badge key={mode} variant="secondary">
									{mode}
								</Badge>
							))
						) : (
							<Badge variant="outline">no mode allow-list</Badge>
						)}
					</div>
				</div>
			</CardHeader>
			<CardContent>
				<button
					type="button"
					className={cn(
						'flex cursor-pointer items-center gap-2 rounded-md text-sm',
						'text-muted-foreground transition-colors focus-ring',
						'duration-[var(--dur-fast)] hover:text-foreground',
					)}
					onClick={() => setSchemaOpen((open) => !open)}
				>
					{schemaOpen ? (
						<ChevronDown className="h-4 w-4" />
					) : (
						<ChevronRight className="h-4 w-4" />
					)}
					Parameters schema
				</button>
				{schemaOpen && (
					<pre className="mt-3 overflow-x-auto rounded-md border bg-muted/40 p-3 font-mono text-xs">
						{formatParameters(tool.parameters)}
					</pre>
				)}
			</CardContent>
		</Card>
	)
}

export function SystemToolsPage() {
	const [tools, setTools] = useState<SystemTool[]>([])
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)
	const [filter, setFilter] = useState('')

	useEffect(() => {
		listSystemTools()
			.then(setTools)
			.catch((err) => {
				setTools([])
				setError(extractErrorMessage(err))
			})
			.finally(() => setLoading(false))
	}, [])

	const filteredTools = useMemo(() => {
		const query = filter.trim().toLowerCase()
		if (!query) return tools
		return tools.filter(
			(tool) =>
				tool.name.toLowerCase().includes(query) ||
				tool.description.toLowerCase().includes(query) ||
				tool.modes.some((mode) => mode.toLowerCase().includes(query)),
		)
	}, [tools, filter])

	return (
		<div className="space-y-6">
			<div>
				<h2 className="text-2xl font-bold tracking-tight">System Tools</h2>
				<p className="text-sm text-muted-foreground">
					Developer catalog of built-in agent tools. This page is not linked
					from the UI.
				</p>
			</div>

			{error && (
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{error}
				</div>
			)}

			<Input
				placeholder="Filter by name, description, or mode..."
				value={filter}
				onChange={(e) => setFilter(e.target.value)}
			/>

			{loading ? (
				<p className="py-8 text-center text-sm text-muted-foreground">
					Loading...
				</p>
			) : filteredTools.length === 0 ? (
				<div className="flex flex-col items-center gap-3 py-8 text-muted-foreground">
					<Wrench className="h-10 w-10" />
					<p className="text-sm">No tools match the current filter.</p>
				</div>
			) : (
				<div className="space-y-4">
					<p className="text-sm text-muted-foreground">
						{filteredTools.length} tool
						{filteredTools.length === 1 ? '' : 's'}
					</p>
					{filteredTools.map((tool) => (
						<ToolCard key={tool.name} tool={tool} />
					))}
				</div>
			)}
		</div>
	)
}
