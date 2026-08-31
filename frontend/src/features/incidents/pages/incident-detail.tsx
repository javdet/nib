import { useCallback, useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router'
import { ArrowLeft, ChevronDown, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import { getDialog, type Dialog } from '@/features/dialogs/api/dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { useMode } from '@/features/modes/mode-context'

export function IncidentDetail() {
	const { id } = useParams<{ id: string }>()
	const navigate = useNavigate()
	const { setActiveDialogId, dialogsVersion } = useDialog()
	const { selectMode } = useMode()
	const [dialog, setDialog] = useState<Dialog | null>(null)
	const [loading, setLoading] = useState(true)
	const [error, setError] = useState<string | null>(null)
	const [summaryExpanded, setSummaryExpanded] = useState(true)

	const loadDetail = useCallback(async (dialogId: string) => {
		setLoading(true)
		setError(null)
		try {
			const dialogData = await getDialog(dialogId)
			setDialog(dialogData)
			selectMode(dialogData.mode)
		} catch (err) {
			setDialog(null)
			setError(
				err instanceof Error ? err.message : 'Failed to load incident',
			)
		} finally {
			setLoading(false)
		}
	}, [selectMode])

	useEffect(() => {
		if (!id) return
		setActiveDialogId(id)
		void loadDetail(id)
	}, [id, setActiveDialogId, loadDetail, dialogsVersion])

	const handleBack = useCallback(() => {
		void navigate('/incidents')
	}, [navigate])

	if (!id) {
		return (
			<div className="text-sm text-muted-foreground">
				No incident selected.
			</div>
		)
	}

	if (loading) {
		return (
			<p className="py-4 text-center text-sm text-muted-foreground">
				Loading incident...
			</p>
		)
	}

	if (error || !dialog) {
		return (
			<div className="space-y-4">
				<Button variant="ghost" size="sm" onClick={handleBack}>
					<ArrowLeft className="mr-2 h-4 w-4" />
					Back to list
				</Button>
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{error ?? 'Incident not found'}
				</div>
			</div>
		)
	}

	return (
		<div className="space-y-6">
			<Button variant="ghost" size="sm" onClick={handleBack}>
				<ArrowLeft className="mr-2 h-4 w-4" />
				Back to list
			</Button>

			<div className="text-center">
				<h2
					className={cn(
						'text-2xl font-bold tracking-tight',
						!dialog.title && 'font-mono text-lg',
					)}
					title={dialog.title || dialog.id}
				>
					{dialog.title || dialog.id}
				</h2>
			</div>

			<Card>
				<CardHeader>
					<div
						role="button"
						tabIndex={0}
						aria-expanded={summaryExpanded}
						className="flex cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
						onClick={() => setSummaryExpanded((prev) => !prev)}
						onKeyDown={(e) => {
							if (e.key === 'Enter' || e.key === ' ') {
								e.preventDefault()
								setSummaryExpanded((prev) => !prev)
							}
						}}
					>
						{summaryExpanded ? (
							<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
						) : (
							<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
						)}
						<CardTitle>Summary</CardTitle>
					</div>
				</CardHeader>
				{summaryExpanded && (
					<CardContent>
						<p className="text-sm text-muted-foreground">
							No summary yet for this conversation.
						</p>
					</CardContent>
				)}
			</Card>
		</div>
	)
}
