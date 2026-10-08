import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/button'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { extractErrorMessage } from '@/lib/api-client'
import { fromLocalInputValue, toLocalInputValue } from '../lib/incident-time'

interface CloseIncidentDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	/** The resolution time already recorded, if any. */
	resolvedAt: string
	onConfirm: (resolvedAt: string) => Promise<void>
}

export function CloseIncidentDialog({
	open,
	onOpenChange,
	resolvedAt,
	onConfirm,
}: CloseIncidentDialogProps) {
	const [value, setValue] = useState('')
	const [busy, setBusy] = useState(false)
	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		if (open) {
			setValue(toLocalInputValue(resolvedAt))
			setError(null)
		}
	}, [open, resolvedAt])

	const handleConfirm = async () => {
		setBusy(true)
		setError(null)
		try {
			await onConfirm(fromLocalInputValue(value))
			onOpenChange(false)
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setBusy(false)
		}
	}

	return (
		<Dialog
			open={open}
			onOpenChange={(next) => {
				if (!busy) onOpenChange(next)
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>Close this incident?</DialogTitle>
					<DialogDescription>
						The impact is assessed once more over the whole incident, then a closure summary is
						written for the post-mortem. The incident is marked closed once both are done, and
						agents can no longer change the post-mortem after that.
					</DialogDescription>
				</DialogHeader>
				<label className="space-y-1 text-sm">
					<span className="text-muted-foreground">Resolved at (leave empty for now)</span>
					<Input
						type="datetime-local"
						value={value}
						onChange={(e) => setValue(e.target.value)}
						disabled={busy}
					/>
				</label>
				{error && <p className="text-sm text-destructive">{error}</p>}
				<DialogFooter>
					<Button type="button" variant="outline" onClick={() => onOpenChange(false)} disabled={busy}>
						Keep open
					</Button>
					<Button type="button" onClick={() => void handleConfirm()} disabled={busy}>
						{busy ? 'Closing...' : 'Close incident'}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
