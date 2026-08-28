import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router'
import { AlertTriangle, Plus, Search, Trash2, X } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { ConfirmDeleteDialog } from '@/components/ui/confirm-delete-dialog'
import { IconButton } from '@/components/ui/icon-button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Pagination } from '@/components/ui/pagination'
import { cn, formatDialogDate } from '@/lib/utils'
import { useMode } from '@/features/modes/mode-context'
import {
	listDialogsPaged,
	createDialog,
	deleteDialog,
	type Dialog,
} from '@/features/dialogs/api/dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { dialogDisplayTitle } from '@/features/dialogs/lib/dialog-title'

const PAGE_SIZE = 10
const INCIDENT_MODE = 'incident'

export function IncidentsPage() {
	const navigate = useNavigate()
	const [dialogs, setDialogs] = useState<Dialog[]>([])
	const [page, setPage] = useState(1)
	const [total, setTotal] = useState(0)
	const [searchInput, setSearchInput] = useState('')
	const [activeQuery, setActiveQuery] = useState('')
	const [loading, setLoading] = useState(true)
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

	const loadDialogs = useCallback(() => {
		setLoading(true)
		listDialogsPaged(
			page,
			PAGE_SIZE,
			undefined,
			INCIDENT_MODE,
			activeQuery || undefined,
		)
			.then(({ items, total: totalCount }) => {
				setDialogs(items)
				setTotal(totalCount)
				if (items.length === 0 && page > 1 && totalCount > 0) {
					const lastPage = Math.ceil(totalCount / PAGE_SIZE)
					setPage(lastPage)
				}
			})
			.catch(() => {
				setDialogs([])
				setTotal(0)
			})
			.finally(() => setLoading(false))
	}, [page, activeQuery])

	useEffect(() => {
		loadDialogs()
	}, [loadDialogs, dialogsVersion])

	const handleNewIncident = useCallback(async () => {
		setError(null)
		setCreating(true)
		try {
			selectMode(INCIDENT_MODE)
			const created = await createDialog({ mode: INCIDENT_MODE })
			setActiveDialogId(created.id)
			setPage(1)
			bumpDialogsVersion()
			void navigate(`/incidents/${created.id}`)
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to create incident',
			)
		} finally {
			setCreating(false)
		}
	}, [selectMode, setActiveDialogId, bumpDialogsVersion, navigate])

	const handleSelect = useCallback(
		(dialog: Dialog) => {
			selectMode(dialog.mode)
			setActiveDialogId(dialog.id)
			void navigate(`/incidents/${dialog.id}`)
		},
		[selectMode, setActiveDialogId, navigate],
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
			if (dialogs.length === 1 && page > 1) {
				setPage((p) => p - 1)
			} else {
				bumpDialogsVersion()
			}
			setDeleteTarget(null)
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to delete incident',
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
	])

	return (
		<div className="space-y-6">
			<h2 className="text-2xl font-bold tracking-tight">Incidents</h2>
			<p className="text-sm text-muted-foreground">
				Select an incident to view its details and continue the conversation
				in the chat panel. New incidents start in incident mode.
			</p>

			{error && (
				<div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
					{error}
				</div>
			)}

			<Card>
				<CardHeader>
					<div className="flex items-center justify-between">
						<CardTitle>Recent incidents</CardTitle>
						<Button
							size="sm"
							onClick={() => void handleNewIncident()}
							disabled={creating}
						>
							<Plus className="mr-2 h-4 w-4" />
							New incident
						</Button>
					</div>
				</CardHeader>
				<CardContent>
					<form onSubmit={handleSearch} className="mb-4 flex gap-2">
						<Input
							type="search"
							placeholder="Search incidents by name..."
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

					{loading ? (
						<p className="py-4 text-center text-sm text-muted-foreground">
							Loading...
						</p>
					) : dialogs.length === 0 ? (
						<div className="flex flex-col items-center gap-2 py-4 text-muted-foreground">
							<AlertTriangle className="h-8 w-8" />
							<p className="text-sm">
								{activeQuery
									? 'No incidents match your search.'
									: 'No incidents yet.'}
							</p>
						</div>
					) : (
						<div className="space-y-2">
							{dialogs.map((dialog) => (
								<div
									key={dialog.id}
									role="button"
									tabIndex={0}
									onClick={() => handleSelect(dialog)}
									onKeyDown={(e) => {
										if (e.key === 'Enter' || e.key === ' ') {
											e.preventDefault()
											handleSelect(dialog)
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
										<AlertTriangle className="h-4 w-4 shrink-0 text-muted-foreground" />
										<span
											className="truncate text-sm"
											title={dialogDisplayTitle(dialog)}
										>
											{dialogDisplayTitle(dialog)}
										</span>
									</div>
									<div className="flex items-center gap-1">
										<span className="shrink-0 whitespace-nowrap text-xs text-muted-foreground">
											{formatDialogDate(dialog.createdAt)}
										</span>
										<IconButton
											variant="ghost"
											size="sm"
											tooltip="Delete incident"
											onClick={(e) => handleDeleteClick(dialog, e)}
										>
											<Trash2 className="h-4 w-4" />
										</IconButton>
									</div>
								</div>
							))}
						</div>
					)}
					<Pagination
						page={page}
						total={total}
						pageSize={PAGE_SIZE}
						onPageChange={setPage}
					/>
				</CardContent>
			</Card>

			<ConfirmDeleteDialog
				open={deleteTarget !== null}
				onOpenChange={(open) => {
					if (!open) setDeleteTarget(null)
				}}
				title="Delete incident"
				itemName={
					deleteTarget ? dialogDisplayTitle(deleteTarget) : ''
				}
				entityLabel="incident"
				deleting={deleting}
				onConfirm={handleConfirmDelete}
			/>
		</div>
	)
}
