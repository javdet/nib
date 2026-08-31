import { useState } from 'react'
import { Wrench, ChevronDown, ChevronRight, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

export interface MCPToolItem {
	name: string
	description: string
	inputSchema?: Record<string, unknown>
}

interface MCPToolsListProps {
	tools: MCPToolItem[]
	loading: boolean
	error: string | null
	onRefresh: () => void
}

export function MCPToolsList({
	tools,
	loading,
	error,
	onRefresh,
}: MCPToolsListProps) {
	const [expandedTool, setExpandedTool] = useState<string | null>(null)

	if (loading) {
		return (
			<div className="flex items-center justify-center py-8">
				<p className="text-sm text-muted-foreground">Loading tools...</p>
			</div>
		)
	}

	if (error) {
		return (
			<div className="space-y-3">
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive break-words whitespace-pre-wrap">
					{error}
				</div>
				<Button variant="outline" size="sm" onClick={onRefresh}>
					Retry
				</Button>
			</div>
		)
	}

	if (tools.length === 0) {
		return (
			<div className="flex flex-col items-center gap-2 py-4 text-muted-foreground">
				<Wrench className="h-8 w-8" />
				<p className="text-sm">No tools available.</p>
			</div>
		)
	}

	return (
		<div className="space-y-2">
			<div className="flex items-center justify-between">
				<p className="text-sm text-muted-foreground">
					{tools.length} tool{tools.length !== 1 ? 's' : ''} available
				</p>
				<Button variant="outline" size="sm" onClick={onRefresh}>
					Refresh
				</Button>
			</div>

			{tools.map((tool) => {
				const isExpanded = expandedTool === tool.name
				return (
					<Card key={tool.name}>
						<CardHeader
							className="cursor-pointer py-3"
							onClick={() =>
								setExpandedTool(isExpanded ? null : tool.name)
							}
						>
							<div className="flex items-center gap-2">
								{isExpanded ? (
									<ChevronDown className="h-4 w-4 text-muted-foreground" />
								) : (
									<ChevronRight className="h-4 w-4 text-muted-foreground" />
								)}
								<CardTitle className="text-sm">{tool.name}</CardTitle>
								<Badge variant="secondary" className="text-xs">
									<Play className="mr-1 h-3 w-3" />
									Tool
								</Badge>
							</div>
						</CardHeader>
						{isExpanded && (
							<CardContent className="pt-0">
								<p className="text-sm text-muted-foreground">
									{tool.description || 'No description available.'}
								</p>
								{tool.inputSchema && (
									<details className="mt-3">
										<summary className="cursor-pointer text-xs font-medium text-muted-foreground">
											Input Schema
										</summary>
										<pre className="mt-2 overflow-x-auto rounded-md bg-muted p-3 text-xs">
											{JSON.stringify(tool.inputSchema, null, 2)}
										</pre>
									</details>
								)}
							</CardContent>
						)}
					</Card>
				)
			})}
		</div>
	)
}
