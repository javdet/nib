import { KeyValueDialog } from '@/features/variables/components/key-value-dialog'
import type { Secret, SecretInput } from '../api/secrets'

interface SecretDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onSave: (input: SecretInput) => void
	initialSecret?: Secret | null
}

export function SecretDialog({
	open,
	onOpenChange,
	onSave,
	initialSecret,
}: SecretDialogProps) {
	return (
		<KeyValueDialog
			open={open}
			onOpenChange={onOpenChange}
			kind="secret"
			onSave={onSave}
			initial={
				initialSecret
					? {
							scope: initialSecret.scope,
							name: initialSecret.name,
							description: initialSecret.description,
							value: '',
						}
					: null
			}
		/>
	)
}
