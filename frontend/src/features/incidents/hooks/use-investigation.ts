import { useCallback, useEffect, useState } from 'react'
import type { AgentActivity } from '@/features/dialogs/api/dialogs'
import { getInvestigation, type IncidentRun } from '../api/incidents'

/**
 * The incident's latest run. It opens no stream of its own: the page passes
 * `handleActivity` to the one usePostmortem already holds.
 */
export function useInvestigation(id: string | undefined) {
	const [run, setRun] = useState<IncidentRun | null>(null)

	const refresh = useCallback(() => {
		if (!id) return
		getInvestigation(id)
			.then(setRun)
			.catch(() => {})
	}, [id])

	useEffect(() => {
		setRun(null)
		refresh()
	}, [refresh])

	const handleActivity = useCallback(
		(ev: AgentActivity) => {
			if (ev.kind === 'investigation_updated' || ev.kind === 'investigation_done') refresh()
		},
		[refresh],
	)

	return { run, refresh, handleActivity }
}
