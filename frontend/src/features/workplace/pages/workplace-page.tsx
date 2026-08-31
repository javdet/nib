import { useState, useCallback } from 'react'
import { useNavigate } from 'react-router'
import { ClipboardList, Plus, Search, Star, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ConfirmDeleteDialog } from '@/components/ui/confirm-delete-dialog'
import { IconButton } from '@/components/ui/icon-button'
import { Card, CardContent } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { cn, formatDialogDate } from '@/lib/utils'
import { useMode } from '@/features/modes/mode-context'
import {
	createDialog,
	deleteDialog,
	setDialogPinned,
	type Dialog,
} from '@/features/dialogs/api/dialogs'
import { usePagedDialogs } from '@/features/dialogs/hooks/use-paged-dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { dialogDisplayTitle } from '@/features/dialogs/lib/dialog-title'
import { PlanStatusBadge } from '@/features/workplace/components/plan-status-badge'

const PAGE_SIZE = 10

interface PlanRowProps {
	dialog: Dialog
	activeDialogId: string | null
	onSelect: (dialog: Dialog) => void
	onPin: (id: string, pinned: boolean, e: React.MouseEvent) => void
	onDelete: (dialog: Dialog, e: React.MouseEvent) => void
}

function PlanRow({
	dialog,
	activeDialogId,
	onSelect,
	onPin,
	onDelete,
}: PlanRowProps) {
	return (
		<div
			key={dialog.id}
			role="button"
			tabIndex={0}
			onClick={() => onSelect(dialog)}
			onKeyDown={(e) => {
				if (e.key === 'Enter' || e.key === ' ') {
					e.preventDefault()
					onSelect(dialog)
				}
			}}
			className={cn(
				'flex cursor-pointer items-center justify-between rounded-lg border p-3 transition-colors',
				activeDialogId === dialog.id
					? 'border-primary bg-primary/5'
					: 'hover:bg-muted/50',
			)}
		>
			<div className="flex min-w-0 items-center gap-2">
				<ClipboardList className="h-4 w-4 shrink-0 text-muted-foreground" />
				<span
					className="truncate text-sm"
					title={dialogDisplayTitle(dialog)}
				>
					{dialogDisplayTitle(dialog)}
				</span>
			</div>
			<div className="flex items-center gap-1">
				{dialog.planStatus ? (
					<PlanStatusBadge status={dialog.planStatus} compact />
				) : null}
				<span className="shrink-0 whitespace-nowrap text-xs text-muted-foreground">
					{formatDialogDate(dialog.createdAt)}
				</span>
				<IconButton
					variant="ghost"
					size="sm"
					tooltip={dialog.pinned ? 'Unpin plan' : 'Pin plan'}
					onClick={(e) => onPin(dialog.id, !dialog.pinned, e)}
				>
					<Star
						className={cn(
							'h-4 w-4',
							dialog.pinned
								? 'fill-primary text-primary'
								: 'text-muted-foreground',
						)}
					/>
				</IconButton>
				<IconButton
					variant="ghost"
					size="sm"
					tooltip="Delete plan"
					onClick={(e) => onDelete(dialog, e)}
				>
					<Trash2 className="h-4 w-4" />
				</IconButton>
			</div>
		</div>
	)
}

export function WorkplacePage() {
	const navigate = useNavigate()
	const [searchInput, setSearchInput] = useState('')
	const [activeQuery, setActiveQuery] = useState('')
	const [creating, setCreating] = useState(false)
	const [deleting, setDeleting] = useState(false)
	const [deleteTarget, setDeleteTarget] = useState<Dialog | null>(null)
	const [error, setError] = useState<string | null>(null)
	const {
		activeDialogId,
		setActiveDialogId,
		dialogsVersion,
		bumpDialogsVersion,
	} = useDialog()
	const { selectMode } = useMode()

	const {
		dialogs,
		pinnedDialogs,
		page,
		total,
		loading,
		setPage,
	} = usePagedDialogs({
		pageSize: PAGE_SIZE,
		search: activeQuery,
		refreshKey: dialogsVersion,
	})

	const handleNewPlan = useCallback(async () => {
		setError(null)
		setCreating(true)
		try {
			selectMode('decompose')
			const created = await createDialog({ mode: 'decompose' })
			setActiveDialogId(created.id)
			setPage(1)
			bumpDialogsVersion()
			void navigate(`/workplace/${created.id}`)
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to create plan',
			)
		} finally {
			setCreating(false)
		}
	}, [selectMode, setActiveDialogId, bumpDialogsVersion, navigate])

	const handleSelect = useCallback(
		(dialog: Dialog) => {
			selectMode(dialog.mode)
			setActiveDialogId(dialog.id)
			void navigate(`/workplace/${dialog.id}`)
		},
		[selectMode, setActiveDialogId, navigate],
	)

	const handlePin = useCallback(
		async (id: string, pinned: boolean, e: React.MouseEvent) => {
			e.stopPropagation()
			setError(null)
			try {
				await setDialogPinned(id, pinned)
				bumpDialogsVersion()
			} catch (err) {
				setError(
					err instanceof Error ? err.message : 'Failed to update pin',
				)
			}
		},
		[bumpDialogsVersion],
	)

	const handleSearch = useCallback(
		(e: React.FormEvent) => {
			e.preventDefault()
			const query = searchInput.trim()
			setActiveQuery(query)
			setPage(1)
		},
		[searchInput],
	)

	const handleClearSearch = useCallback(() => {
		setSearchInput('')
		setActiveQuery('')
		setPage(1)
	}, [])

	const handleDeleteClick = useCallback(
		(dialog: Dialog, e: React.MouseEvent) => {
			e.stopPropagation()
			setDeleteTarget(dialog)
		},
		[],
	)

	const handleConfirmDelete = useCallback(async () => {
		if (!deleteTarget) return

		const id = deleteTarget.id
		setError(null)
		setDeleting(true)
		try {
			await deleteDialog(id)
			if (activeDialogId === id) {
				setActiveDialogId(null)
			}
			const isPinned = pinnedDialogs.some((d) => d.id === id)
			if (!isPinned && dialogs.length === 1 && page > 1) {
				setPage((p) => p - 1)
			} else {
				bumpDialogsVersion()
			}
			setDeleteTarget(null)
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to delete plan',
			)
		} finally {
			setDeleting(false)
		}
	}, [
		deleteTarget,
		activeDialogId,
		setActiveDialogId,
		bumpDialogsVersion,
		dialogs.length,
		page,
		pinnedDialogs,
	])

	const hasPlans = activeQuery
		? dialogs.length > 0
		: pinnedDialogs.length > 0 || dialogs.length > 0

	return (
		<div className="space-y-6">
			<h2 className="text-2xl font-bold tracking-tight">Plans</h2>
			<p className="text-sm text-muted-foreground">
				Select a plan to view its details and continue the conversation
				in the chat panel. New plans start in decompose mode.
			</p>

			{error && (
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{error}
				</div>
			)}

			<Card>
				<CardContent className="space-y-6 pt-6">
					<div className="flex items-center justify-between gap-4">
						<form onSubmit={handleSearch} className="flex gap-2">
							<Input
								type="search"
								placeholder="Search plans by name..."
								value={searchInput}
								onChange={(e) => setSearchInput(e.target.value)}
								className="max-w-sm"
							/>
							<Button type="submit" variant="secondary" size="sm">
								<Search className="mr-2 h-4 w-4" />
								Search
							</Button>
							{activeQuery && (
								<Button
									type="button"
									variant="ghost"
									size="sm"
									onClick={handleClearSearch}
								>
									<X className="mr-2 h-4 w-4" />
									Clear
								</Button>
							)}
						</form>
						<Button
							size="sm"
							onClick={() => void handleNewPlan()}
							disabled={creating}
						>
							<Plus className="mr-2 h-4 w-4" />
							New plan
						</Button>
					</div>

					{loading ? (
						<p className="py-4 text-center text-sm text-muted-foreground">
							Loading...
						</p>
					) : !hasPlans ? (
						<div className="flex flex-col items-center gap-2 py-4 text-muted-foreground">
							<ClipboardList className="h-8 w-8" />
							<p className="text-sm">
								{activeQuery
									? 'No plans match your search.'
									: 'No plans yet.'}
							</p>
						</div>
					) : activeQuery ? (
						<section className="space-y-2">
							<h3 className="text-sm font-medium text-muted-foreground">
								Search results
							</h3>
							<div className="space-y-2">
								{dialogs.map((dialog) => (
									<PlanRow
										key={dialog.id}
										dialog={dialog}
										activeDialogId={activeDialogId}
										onSelect={handleSelect}
										onPin={handlePin}
										onDelete={handleDeleteClick}
									/>
								))}
							</div>
							<Pagination
								page={page}
								total={total}
								pageSize={PAGE_SIZE}
								onPageChange={setPage}
							/>
						</section>
					) : (
						<>
							{pinnedDialogs.length > 0 && (
								<section className="space-y-2">
									<h3 className="text-sm font-medium text-muted-foreground">
										Pinned
									</h3>
									<div className="space-y-2">
										{pinnedDialogs.map((dialog) => (
											<PlanRow
												key={dialog.id}
												dialog={dialog}
												activeDialogId={activeDialogId}
												onSelect={handleSelect}
												onPin={handlePin}
												onDelete={handleDeleteClick}
											/>
										))}
									</div>
								</section>
							)}

							<section className="space-y-2">
								<h3 className="text-sm font-medium text-muted-foreground">
									Recents
								</h3>
								{dialogs.length === 0 ? (
									<p className="py-2 text-sm text-muted-foreground">
										No recent plans.
									</p>
								) : (
									<div className="space-y-2">
										{dialogs.map((dialog) => (
											<PlanRow
												key={dialog.id}
												dialog={dialog}
												activeDialogId={activeDialogId}
												onSelect={handleSelect}
												onPin={handlePin}
												onDelete={handleDeleteClick}
											/>
										))}
									</div>
								)}
								<Pagination
									page={page}
									total={total}
									pageSize={PAGE_SIZE}
									onPageChange={setPage}
								/>
							</section>
						</>
					)}
				</CardContent>
			</Card>

			<ConfirmDeleteDialog
				open={deleteTarget !== null}
				onOpenChange={(open) => {
					if (!open) setDeleteTarget(null)
				}}
				title="Delete plan"
				itemName={
					deleteTarget ? dialogDisplayTitle(deleteTarget) : ''
				}
				entityLabel="plan"
				deleting={deleting}
				onConfirm={handleConfirmDelete}
			/>
		</div>
	)
}
