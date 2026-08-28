import { Button } from '@/components/ui/button'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'

interface ConfirmDeleteDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	title: string
	itemName: string
	entityLabel: string
	deleting?: boolean
	onConfirm: () => void | Promise<void>
}

export function ConfirmDeleteDialog({
	open,
	onOpenChange,
	title,
	itemName,
	entityLabel,
	deleting = false,
	onConfirm,
}: ConfirmDeleteDialogProps) {
	return (
		<Dialog
			open={open}
			onOpenChange={(nextOpen) => {
				if (deleting) return
				onOpenChange(nextOpen)
			}}
		>
			<DialogContent>
				<DialogHeader>
					<DialogTitle>{title}</DialogTitle>
					<DialogDescription>
						Are you sure you want to delete{' '}
						<span className="font-medium text-foreground">
							{itemName}
						</span>
						? This {entityLabel} and its conversation history will be
						permanently removed. This action cannot be undone.
					</DialogDescription>
				</DialogHeader>
				<DialogFooter>
					<Button
						type="button"
						variant="outline"
						onClick={() => onOpenChange(false)}
						disabled={deleting}
					>
						Cancel
					</Button>
					<Button
						type="button"
						variant="destructive"
						onClick={() => void onConfirm()}
						disabled={deleting}
					>
						{deleting ? 'Deleting...' : 'Delete'}
					</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}
