import { useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import { extractErrorMessage } from '@/lib/api-client'
import { useMode } from '@/features/modes/mode-context'
import { useDialog } from '@/features/dialogs/dialog-context'
import { createDialog } from '@/features/dialogs/api/dialogs'
import {
	listToolCategories,
	listCategoryTools,
	listUncategorizedTools,
	setCategoryPatterns,
	patternsToLines,
	linesToPatterns,
	type ToolCategory,
	type CategorizedTool,
} from '../api/tool-categories'
import { buildCategorizeToolsRequest } from '../lib/build-categorize-request'

type Selection = { kind: 'category'; name: string } | { kind: 'uncategorized' }

export function ToolCategoriesCard() {
	const { selectMode } = useMode()
	const { setActiveDialogId, bumpDialogsVersion, enqueuePendingMessage } =
		useDialog()

	const [categories, setCategories] = useState<ToolCategory[]>([])
	const [selection, setSelection] = useState<Selection | null>(null)
	const [tools, setTools] = useState<CategorizedTool[]>([])
	const [patternText, setPatternText] = useState('')
	const [savedPatternText, setSavedPatternText] = useState('')
	const [loading, setLoading] = useState(true)
	const [toolsLoading, setToolsLoading] = useState(false)
	const [saving, setSaving] = useState(false)
	const [filling, setFilling] = useState(false)
	const [error, setError] = useState<string | null>(null)

	const patternDirty = patternText !== savedPatternText

	const refreshCategories = useCallback(async () => {
		const list = await listToolCategories()
		setCategories(list)
		return list
	}, [])

	useEffect(() => {
		refreshCategories()
			.catch((err) => {
				setError(extractErrorMessage(err))
				setCategories([])
			})
			.finally(() => setLoading(false))
	}, [refreshCategories])

	useEffect(() => {
		if (!selection) {
			setTools([])
			setPatternText('')
			setSavedPatternText('')
			return
		}

		setToolsLoading(true)
		setError(null)

		if (selection.kind === 'uncategorized') {
			listUncategorizedTools()
				.then((list) => setTools(list))
				.catch((err) => {
					setError(extractErrorMessage(err))
					setTools([])
				})
				.finally(() => setToolsLoading(false))
			setPatternText('')
			setSavedPatternText('')
			return
		}

		const category = categories.find((c) => c.name === selection.name)
		const patterns = category?.patterns ?? []
		const text = patternsToLines(patterns)
		setPatternText(text)
		setSavedPatternText(text)

		listCategoryTools(selection.name)
			.then((list) => setTools(list))
			.catch((err) => {
				setError(extractErrorMessage(err))
				setTools([])
			})
			.finally(() => setToolsLoading(false))
	}, [selection, categories])

	const handleSelectCategory = (name: string) => {
		setSelection({ kind: 'category', name })
	}

	const handleSavePatterns = async () => {
		if (!selection || selection.kind !== 'category') return

		setSaving(true)
		setError(null)
		try {
			const updated = await setCategoryPatterns(
				selection.name,
				linesToPatterns(patternText),
			)
			setCategories((prev) =>
				prev.map((c) => (c.name === updated.name ? updated : c)),
			)
			setSavedPatternText(patternText)
			const list = await listCategoryTools(selection.name)
			setTools(list)
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}

	const handleAutoFill = useCallback(async () => {
		setFilling(true)
		setError(null)
		try {
			// Read the bucket fresh rather than reusing the selection's `tools`:
			// the panel on the right holds whatever category is selected, and the
			// skill is only ever given the tools that still have no category.
			const uncategorized = await listUncategorizedTools()
			if (uncategorized.length === 0) {
				setError('No uncategorized tools - every tool already has a category.')
				return
			}

			// Only discuss carries the skills list and the update_tool_category
			// tool, and the system prompt is frozen on a dialog's first message -
			// so the mode has to be settled before anything is sent.
			selectMode('discuss')
			const dialog = await createDialog({
				mode: 'discuss',
				title: 'Tool categories: auto-fill',
			})
			// The chat panel picks this up once it has loaded the (empty)
			// transcript for the newly active dialog and sends it for us.
			enqueuePendingMessage({
				dialogId: dialog.id,
				text: buildCategorizeToolsRequest(categories, uncategorized),
			})
			setActiveDialogId(dialog.id)
			bumpDialogsVersion()
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setFilling(false)
		}
	}, [
		categories,
		selectMode,
		enqueuePendingMessage,
		setActiveDialogId,
		bumpDialogsVersion,
	])

	return (
		<Card>
			<CardHeader className="pb-3">
				<div className="flex items-start justify-between gap-4">
					<CardTitle className="text-base">Tool categories</CardTitle>
					<Button
						size="sm"
						variant="outline"
						className="shrink-0"
						onClick={() => void handleAutoFill()}
						disabled={filling || loading || categories.length === 0}
					>
						{filling ? 'Opening chat...' : 'Auto-fill with AI'}
					</Button>
				</div>
				<p className="text-sm text-muted-foreground">
					Assign tools to categories using exact names or prefix patterns
					(e.g. <span className="font-mono">kubernetes_*</span>). Category
					names are defined in the{' '}
					<Link to="/variables" className="underline underline-offset-2">
						toolCategories
					</Link>{' '}
					variable on the Variables page.
				</p>
				<p className="text-sm text-muted-foreground">
					Auto-fill opens a discuss chat that sorts the uncategorized tools
					into these categories. It <strong>replaces</strong> the patterns of
					every category it assigns tools to, so patterns written by hand in
					those categories are dropped.
				</p>
			</CardHeader>
			<CardContent>
				{error && (
					<div className="mb-4 rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
						{error}
					</div>
				)}

				{loading ? (
					<p className="py-4 text-center text-sm text-muted-foreground">
						Loading...
					</p>
				) : (
					<div className="grid gap-4 md:grid-cols-[220px_minmax(0,1fr)]">
						<div className="space-y-1">
							{categories.map((category) => (
								<button
									key={category.name}
									type="button"
									className={cn(
										'flex w-full cursor-pointer items-center justify-between gap-2',
										'rounded-md border px-3 py-2 text-left text-sm focus-ring',
										'transition-colors duration-[var(--dur-fast)]',
										selection?.kind === 'category' &&
											selection.name === category.name
											? 'border-primary/50 bg-foreground/[0.07]'
											: 'hover:border-ring/40 hover:bg-foreground/[0.04]',
									)}
									onClick={() => handleSelectCategory(category.name)}
								>
									<span className="truncate font-mono">
										{category.name}
									</span>
									<div className="flex shrink-0 gap-1">
										<Badge variant="outline" className="text-xs">
											{category.patterns.length} pat
										</Badge>
										<Badge variant="secondary" className="text-xs">
											{category.toolCount}
										</Badge>
									</div>
								</button>
							))}
							<button
								type="button"
								className={cn(
									'flex w-full cursor-pointer items-center justify-between gap-2',
									'rounded-md border px-3 py-2 text-left text-sm focus-ring',
									'transition-colors duration-[var(--dur-fast)]',
									selection?.kind === 'uncategorized'
										? 'border-primary/50 bg-foreground/[0.07]'
										: 'hover:border-ring/40 hover:bg-foreground/[0.04]',
								)}
								onClick={() => setSelection({ kind: 'uncategorized' })}
							>
								<span>Uncategorized</span>
								<Badge variant="outline" className="text-xs">
									null
								</Badge>
							</button>
						</div>

						<div className="space-y-4">
							{!selection ? (
								<p className="text-sm text-muted-foreground">
									Select a category to edit patterns and view matched
									tools.
								</p>
							) : selection.kind === 'uncategorized' ? (
								<div className="space-y-3">
									<p className="text-sm text-muted-foreground">
										Tools with no category assignment (
										<span className="font-mono">categories: null</span>).
									</p>
									{toolsLoading ? (
										<p className="text-sm text-muted-foreground">
											Loading tools...
										</p>
									) : tools.length === 0 ? (
										<p className="text-sm text-muted-foreground">
											No uncategorized tools.
										</p>
									) : (
										<div className="space-y-2">
											{tools.map((tool) => (
												<div
													key={`${tool.server}/${tool.name}`}
													className="rounded-md border px-3 py-2"
												>
													<div className="font-mono text-sm">
														{tool.server}/{tool.name}
													</div>
													{tool.description && (
														<p className="text-xs text-muted-foreground">
															{tool.description}
														</p>
													)}
												</div>
											))}
										</div>
									)}
								</div>
							) : (
								<>
									<div className="space-y-2">
										<Label htmlFor="category-patterns">
											Patterns for{' '}
											<span className="font-mono">
												{selection.name}
											</span>
										</Label>
										<Textarea
											id="category-patterns"
											rows={8}
											value={patternText}
											onChange={(e) => setPatternText(e.target.value)}
											disabled={saving}
											placeholder="kubernetes_get_pods&#10;kubernetes_*"
										/>
										<p className="text-xs text-muted-foreground">
											One pattern per line. Use trailing{' '}
											<span className="font-mono">*</span> for prefix
											matches.
										</p>
										<div className="flex gap-2">
											<Button
												size="sm"
												onClick={() => void handleSavePatterns()}
												disabled={saving || !patternDirty}
											>
												{saving ? 'Saving...' : 'Save patterns'}
											</Button>
											<Button
												size="sm"
												variant="outline"
												onClick={() => setPatternText(savedPatternText)}
												disabled={saving || !patternDirty}
											>
												Reset
											</Button>
										</div>
									</div>

									<div className="space-y-2">
										<Label>Matched tools</Label>
										{toolsLoading ? (
											<p className="text-sm text-muted-foreground">
												Loading tools...
											</p>
										) : tools.length === 0 ? (
											<p className="text-sm text-muted-foreground">
												No tools match these patterns yet.
											</p>
										) : (
											<div className="space-y-2">
												{tools.map((tool) => (
													<div
														key={`${tool.server}/${tool.name}`}
														className="rounded-md border px-3 py-2"
													>
														<div className="font-mono text-sm">
															{tool.server}/{tool.name}
														</div>
														{tool.description && (
															<p className="text-xs text-muted-foreground">
																{tool.description}
															</p>
														)}
														{tool.categories &&
															tool.categories.length > 0 && (
																<div className="mt-1 flex flex-wrap gap-1">
																	{tool.categories.map((tag) => (
																		<Badge
																			key={tag}
																			variant="secondary"
																			className="text-xs"
																		>
																			{tag}
																		</Badge>
																	))}
																</div>
															)}
													</div>
												))}
											</div>
										)}
									</div>
								</>
							)}
						</div>
					</div>
				)}
			</CardContent>
		</Card>
	)
}
