import { createContext, useContext, useEffect, useId } from 'react'

type SetDirty = (id: string, dirty: boolean) => void

export const UnsavedChangesContext = createContext<SetDirty>(() => {})

export const UNSAVED_CHANGES_MESSAGE =
	'You have unsaved changes. Leave this page and discard them?'

interface PathLike {
	pathname: string
	search: string
}

/**
 * Whether a navigation replaces what is on screen. A tab carried in the query
 * (`/tools?tab=executor`) mounts different cards, so search counts; a hash
 * only scrolls, so it does not.
 */
export function leavesPage(current: PathLike, next: PathLike): boolean {
	return current.pathname !== next.pathname || current.search !== next.search
}

/**
 * Marks the calling component as holding edits that navigating away would
 * lose. Every editor registers with the one guard at the app root, since
 * react-router honours only a single blocker at a time.
 */
export function useUnsavedChanges(dirty: boolean) {
	const setDirty = useContext(UnsavedChangesContext)
	const id = useId()

	useEffect(() => {
		if (!dirty) return
		setDirty(id, true)
		return () => setDirty(id, false)
	}, [dirty, id, setDirty])
}
