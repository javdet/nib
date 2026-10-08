import { useCallback, useEffect, useRef, useState } from 'react'
import { openDialogActivity, type AgentActivity } from '@/features/dialogs/api/dialogs'
import {
	editPostmortem,
	getPostmortem,
	type Postmortem,
	type PostmortemPatch,
} from '../api/incidents'

export interface UsePostmortemResult {
	postmortem: Postmortem | null
	error: string | null
	/** Applies an operator edit and adopts the document the server answers with. */
	edit: (patch: PostmortemPatch) => Promise<void>
	refresh: () => void
}

/**
 * Loads an incident's post-mortem and keeps it current while agents write it.
 *
 * A document older than the one already shown is ignored: an SSE refetch and
 * the answer to the operator's own edit can arrive in either order. Editors
 * keep their draft in their own state, so a refetch never overwrites what the
 * operator is typing.
 */
export function usePostmortem(
	id: string | undefined,
	/** Receives every other event of the stream, so the page opens only one. */
	onActivity?: (ev: AgentActivity) => void,
): UsePostmortemResult {
	const [postmortem, setPostmortem] = useState<Postmortem | null>(null)
	const [error, setError] = useState<string | null>(null)
	const versionRef = useRef(-1)

	const adopt = useCallback((doc: Postmortem) => {
		if (doc.version < versionRef.current) return
		versionRef.current = doc.version
		setPostmortem(doc)
	}, [])

	const refresh = useCallback(() => {
		if (!id) return
		getPostmortem(id)
			.then((doc) => {
				setError(null)
				adopt(doc)
			})
			.catch((err: unknown) => {
				setError(err instanceof Error ? err.message : 'Failed to load the post-mortem')
			})
	}, [id, adopt])

	useEffect(() => {
		versionRef.current = -1
		setPostmortem(null)
		refresh()
	}, [refresh])

	const onActivityRef = useRef(onActivity)
	useEffect(() => {
		onActivityRef.current = onActivity
	}, [onActivity])

	useEffect(() => {
		if (!id) return
		return openDialogActivity(id, (ev) => {
			if (ev.kind === 'postmortem_updated') refresh()
			else onActivityRef.current?.(ev)
		})
	}, [id, refresh])

	const edit = useCallback(
		async (patch: PostmortemPatch) => {
			if (!id) return
			adopt(await editPostmortem(id, patch))
		},
		[id, adopt],
	)

	return { postmortem, error, edit, refresh }
}
