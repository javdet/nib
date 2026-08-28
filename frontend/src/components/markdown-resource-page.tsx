import { type ReactNode } from 'react'
import { FileCode2, Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { FrontmatterEditor } from '@/components/frontmatter-editor'
import { cn } from '@/lib/utils'
import type { NamedMarkdownResource } from '@/lib/hooks/use-named-markdown-resource'

import type { MarkdownListItem } from '@/components/markdown-resource-page-types'

interface MarkdownResourcePageProps {
	title: string
	description: string
	listTitle: string
	addLabel: string
	onAddClick: () => void
	emptyIcon: ReactNode
	emptyListMessage: string
	emptySelectionMessage: string
	loadingSelectionMessage: string
	showCategory?: boolean
	createDialog: ReactNode
	resource: NamedMarkdownResource
	listItems?: MarkdownListItem[]
}

export function MarkdownResourcePage({
	title,
	description,
	listTitle,
	addLabel,
	onAddClick,
	emptyIcon,
	emptyListMessage,
	emptySelectionMessage,
	loadingSelectionMessage,
	showCategory,
	createDialog,
	resource,
	listItems,
}: MarkdownResourcePageProps) {
	const items: MarkdownListItem[] =
		listItems ??
		resource.names.map((name) => ({
			name,
		}))

	return (
		<div className="-m-6 flex h-[calc(100vh-3.5rem)] flex-col gap-4 p-6">
			<div className="flex shrink-0 items-start justify-between gap-4">
				<div>
					<h2 className="text-2xl font-bold tracking-tight">{title}</h2>
					<p className="text-sm text-muted-foreground">{description}</p>
				</div>
				<Button
					size="sm"
					onClick={onAddClick}
					disabled={resource.listLoading || resource.saving}
				>
					<Plus className="mr-2 h-4 w-4" />
					{addLabel}
				</Button>
			</div>

			{resource.error && (
				<div className="shrink-0 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
					{resource.error}
				</div>
			)}

			<div className="flex min-h-0 flex-1 gap-4 rounded-lg border">
				<aside className="flex w-56 shrink-0 flex-col border-r bg-muted/30">
					<div className="border-b px-3 py-2 text-xs font-medium text-muted-foreground">
						{listTitle}
					</div>
					<div className="min-h-0 flex-1 overflow-y-auto p-2">
						{resource.listLoading ? (
							<p className="px-2 py-4 text-center text-sm text-muted-foreground">
								Loading...
							</p>
						) : items.length === 0 ? (
							<div className="flex flex-col items-center gap-2 px-2 py-6 text-muted-foreground">
								{emptyIcon}
								<p className="text-center text-xs">{emptyListMessage}</p>
							</div>
						) : (
							<ul className="space-y-1">
								{items.map((item) => {
									const selected = item.name === resource.selectedName
									return (
										<li key={item.name}>
											<button
												type="button"
												onClick={() => resource.selectName(item.name)}
												className={cn(
													'flex w-full items-center justify-between gap-2 rounded-md px-2 py-1.5 text-left text-sm',
													selected
														? 'bg-accent text-accent-foreground'
														: 'hover:bg-accent/50',
												)}
											>
												<span className="truncate font-mono">
													{item.name}.md
												</span>
												{item.badge ? (
													<Badge
														variant="default"
														className="shrink-0 text-[10px]"
													>
														{item.badge}
													</Badge>
												) : null}
											</button>
										</li>
									)
								})}
							</ul>
						)}
					</div>
				</aside>

				<div className="flex min-h-0 min-w-0 flex-1 flex-col p-4">
					{resource.selectedName ? (
						<FrontmatterEditor
							fileName={resource.selectedName}
							content={resource.content}
							isDirty={resource.isDirty}
							loading={resource.contentLoading}
							saving={resource.saving}
							showCategory={showCategory}
							onChange={resource.setContent}
							onSave={() => void resource.save()}
							onDelete={() => void resource.remove()}
						/>
					) : (
						<div className="flex flex-1 flex-col items-center justify-center gap-2 text-muted-foreground">
							<FileCode2 className="h-10 w-10" />
							<p className="text-sm">
								{resource.listLoading
									? loadingSelectionMessage
									: emptySelectionMessage}
							</p>
						</div>
					)}
				</div>
			</div>

			{createDialog}
		</div>
	)
}
