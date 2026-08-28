import { useCallback, useEffect, useState } from 'react'
import {
	ChevronDown,
	CornerDownRight,
	Loader2,
	MessageSquare,
	Plus,
} from 'lucide-react'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { IconButton } from '@/components/ui/icon-button'
import { cn } from '@/lib/utils'
import { extractErrorMessage } from '@/lib/api-client'
import { useDialog } from '@/features/dialogs/dialog-context'
import {
	getDialog,
	listDialogsPaged,
	type Dialog,
} from '@/features/dialogs/api/dialogs'
import { DEFAULT_MODE, useMode } from '@/features/modes/mode-context'
import { dialogDisplayTitle } from '@/features/dialogs/lib/dialog-title'

const PAGE_SIZE = 10

export function ChatHistoryMenu() {
	const {
		activeDialogId,
		setActiveDialogId,
		dialogsVersion,
	} = useDialog()
	const { selectMode } = useMode()
	const [open, setOpen] = useState(false)
	const [dialogs, setDialogs] = useState<Dialog[]>([])
	const [page, setPage] = useState(1)
	const [total, setTotal] = useState(0)
	const [loading, setLoading] = useState(false)
	const [loadingMore, setLoadingMore] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [activeDialogTitle, setActiveDialogTitle] = useState<string | null>(
		null,
	)

	useEffect(() => {
		if (!activeDialogId) {
			setActiveDialogTitle(null)
			selectMode(DEFAULT_MODE)
			return
		}

		let cancelled = false
		getDialog(activeDialogId)
			.then((dialog) => {
				if (!cancelled) {
					setActiveDialogTitle(dialogDisplayTitle(dialog))
					selectMode(dialog.mode)
				}
			})
			.catch(() => {
				if (!cancelled) {
					setActiveDialogTitle(null)
				}
			})

		return () => {
			cancelled = true
		}
	}, [activeDialogId, dialogsVersion, selectMode])

	const loadPage = useCallback(async (pageNum: number, append: boolean) => {
		if (append) {
			setLoadingMore(true)
		} else {
			setLoading(true)
		}
		setError(null)

		try {
			const { items, total: totalCount } = await listDialogsPaged(
				pageNum,
				PAGE_SIZE,
				'all',
			)
			setTotal(totalCount)
			setPage(pageNum)
			setDialogs((prev) => (append ? [...prev, ...items] : items))
		} catch (err) {
			setError(extractErrorMessage(err))
			if (!append) {
				setDialogs([])
				setTotal(0)
			}
		} finally {
			setLoading(false)
			setLoadingMore(false)
		}
	}, [])

	useEffect(() => {
		if (!open) return
		void loadPage(1, false)
	}, [open, loadPage, dialogsVersion])

	const handleLoadMore = useCallback(() => {
		if (loadingMore || loading) return
		void loadPage(page + 1, true)
	}, [loadPage, page, loadingMore, loading])

	const handleSelect = useCallback(
		(dialog: Dialog) => {
			selectMode(dialog.mode)
			setActiveDialogId(dialog.id)
			setOpen(false)
		},
		[selectMode, setActiveDialogId],
	)

	const handleNewDiscuss = useCallback(() => {
		selectMode('discuss')
		setActiveDialogId(null)
		setOpen(false)
	}, [selectMode, setActiveDialogId])

	const hasMore = dialogs.length < total

	return (
		<DropdownMenu open={open} onOpenChange={setOpen}>
			<div className="flex shrink-0 border-b">
				<DropdownMenuTrigger asChild>
					<button
						type="button"
						className="flex min-w-0 flex-1 items-center gap-2 px-4 py-3 text-left outline-none hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring"
					>
						<MessageSquare className="h-4 w-4 shrink-0 text-muted-foreground" />
						<span className="shrink-0 text-sm font-medium">Chat</span>
						{activeDialogId && (
							<span
								className="min-w-0 flex-1 truncate text-xs text-muted-foreground"
								title={activeDialogTitle || activeDialogId}
							>
								{activeDialogTitle || activeDialogId}
							</span>
						)}
						<ChevronDown
							className={cn(
								'ml-auto h-4 w-4 shrink-0 text-muted-foreground transition-transform',
								open && 'rotate-180',
							)}
						/>
					</button>
				</DropdownMenuTrigger>
				<IconButton
					type="button"
					variant="outline"
					onClick={handleNewDiscuss}
					tooltip="New chat"
					className="my-2 mr-2 h-8 w-8 shrink-0 border-border bg-muted/60 shadow-sm hover:bg-muted"
				>
					<Plus className="h-4 w-4" />
				</IconButton>
			</div>
			<DropdownMenuContent
				align="start"
				sideOffset={0}
				className="w-[var(--radix-dropdown-menu-trigger-width)] p-0"
			>
				<div className="max-h-80 overflow-y-auto">
					{loading && dialogs.length === 0 && (
						<div className="flex items-center gap-2 px-3 py-4 text-sm text-muted-foreground">
							<Loader2 className="h-3.5 w-3.5 animate-spin" />
							Loading chats…
						</div>
					)}

					{error && (
						<div className="px-3 py-4 text-xs text-destructive">
							{error}
						</div>
					)}

					{!loading && !error && dialogs.length === 0 && (
						<p className="px-3 py-4 text-sm text-muted-foreground">
							No chats yet.
						</p>
					)}

					{dialogs.map((dialog) => {
						const isNested = Boolean(dialog.parentId)
						const isActive = dialog.id === activeDialogId

						return (
							<button
								key={dialog.id}
								type="button"
								onClick={() => handleSelect(dialog)}
								className={cn(
									'flex w-full items-center gap-2 px-3 py-2 text-left text-sm transition-colors hover:bg-accent',
									isActive && 'bg-accent/60',
									isNested && 'pl-6',
								)}
							>
								{isNested && (
									<CornerDownRight className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
								)}
								<span
									className="min-w-0 flex-1 truncate"
									title={dialogDisplayTitle(dialog)}
								>
									{dialogDisplayTitle(dialog)}
								</span>
							</button>
						)
					})}
				</div>

				{hasMore && (
					<div className="border-t p-2">
						<Button
							type="button"
							variant="ghost"
							size="sm"
							className="w-full"
							disabled={loadingMore || loading}
							onClick={handleLoadMore}
						>
							{loadingMore ? (
								<>
									<Loader2 className="mr-2 h-3.5 w-3.5 animate-spin" />
									Loading…
								</>
							) : (
								'Load 10 more'
							)}
						</Button>
					</div>
				)}
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
