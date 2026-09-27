import { useCallback, useRef, useState, type ChangeEvent } from 'react'
import { useNavigate } from 'react-router'
import { extractErrorMessage } from '@/lib/api-client'
import { importPlanBundle } from '@/features/dialogs/api/dialogs'
import { useDialog } from '@/features/dialogs/dialog-context'
import { useMode } from '@/features/modes/mode-context'
import { parsePlanBundle, PLAN_BUNDLE_EXTENSION } from '../lib/plan-bundle'

export const planImportAccept = `${PLAN_BUNDLE_EXTENSION},application/json`

/**
 * Drives "Import from .nib": a hidden file input the caller renders, and the
 * upload that turns the picked file into a new plan and opens it.
 */
export function usePlanImport(onError: (message: string | null) => void) {
	const navigate = useNavigate()
	const { setActiveDialogId, bumpDialogsVersion } = useDialog()
	const { selectMode } = useMode()
	const inputRef = useRef<HTMLInputElement>(null)
	const [importing, setImporting] = useState(false)

	const openFilePicker = useCallback(() => {
		inputRef.current?.click()
	}, [])

	const handleFileChange = useCallback(
		async (e: ChangeEvent<HTMLInputElement>) => {
			const file = e.target.files?.[0]
			// Cleared so picking the same file again after a failure fires
			// another change event.
			e.target.value = ''
			if (!file) return

			onError(null)
			setImporting(true)
			try {
				const bundle = parsePlanBundle(await file.text())
				const created = await importPlanBundle(bundle)
				selectMode(created.mode)
				setActiveDialogId(created.id)
				bumpDialogsVersion()
				void navigate(`/workplace/${created.id}`)
			} catch (err) {
				onError(extractErrorMessage(err))
			} finally {
				setImporting(false)
			}
		},
		[onError, selectMode, setActiveDialogId, bumpDialogsVersion, navigate],
	)

	return { inputRef, importing, openFilePicker, handleFileChange }
}
