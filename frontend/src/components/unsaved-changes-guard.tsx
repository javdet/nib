import { useCallback, useEffect, useState, type ReactNode } from 'react'
import { useBlocker, type BlockerFunction } from 'react-router'
import {
	UNSAVED_CHANGES_MESSAGE,
	UnsavedChangesContext,
	leavesPage,
} from '@/lib/unsaved-changes'

/**
 * Asks before discarding edits that a component registered with
 * `useUnsavedChanges`: on in-app navigation through the router's blocker, and
 * on reload or tab close through `beforeunload`, where the browser shows its
 * own wording.
 */
export function UnsavedChangesGuard({ children }: { children: ReactNode }) {
	const [dirtyIds, setDirtyIds] = useState<ReadonlySet<string>>(new Set())
	const hasUnsaved = dirtyIds.size > 0

	const setDirty = useCallback((id: string, dirty: boolean) => {
		setDirtyIds((prev) => {
			if (prev.has(id) === dirty) return prev
			const next = new Set(prev)
			if (dirty) next.add(id)
			else next.delete(id)
			return next
		})
	}, [])

	const shouldBlock = useCallback<BlockerFunction>(
		({ currentLocation, nextLocation }) =>
			hasUnsaved && leavesPage(currentLocation, nextLocation),
		[hasUnsaved],
	)
	const blocker = useBlocker(shouldBlock)

	useEffect(() => {
		if (blocker.state !== 'blocked') return
		if (window.confirm(UNSAVED_CHANGES_MESSAGE)) blocker.proceed()
		else blocker.reset()
	}, [blocker])

	useEffect(() => {
		if (!hasUnsaved) return
		function handleBeforeUnload(e: BeforeUnloadEvent) {
			e.preventDefault()
			// Chrome before 119 still requires returnValue to show the prompt.
			e.returnValue = ''
		}
		window.addEventListener('beforeunload', handleBeforeUnload)
		return () => window.removeEventListener('beforeunload', handleBeforeUnload)
	}, [hasUnsaved])

	return (
		<UnsavedChangesContext.Provider value={setDirty}>
			{children}
		</UnsavedChangesContext.Provider>
	)
}
