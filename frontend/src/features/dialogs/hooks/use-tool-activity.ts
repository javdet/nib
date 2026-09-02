import { useEffect, useReducer, useRef } from 'react'
import {
	openDialogActivity,
	type AgentActivity,
} from '@/features/dialogs/api/dialogs'
import {
	hideDelay,
	initialToolActivityState,
	reduceToolActivity,
} from '@/features/dialogs/lib/tool-activity'

export function useToolActivity(
	dialogId: string | null,
	onAgentResult?: () => void,
	onActionResult?: () => void,
): boolean {
	const [state, dispatch] = useReducer(
		reduceToolActivity,
		initialToolActivityState,
	)
	const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
	const shownAtRef = useRef<number | null>(null)
	const onAgentResultRef = useRef(onAgentResult)
	const onActionResultRef = useRef(onActionResult)

	useEffect(() => {
		onAgentResultRef.current = onAgentResult
	}, [onAgentResult])

	useEffect(() => {
		onActionResultRef.current = onActionResult
	}, [onActionResult])

	const clearHideTimer = () => {
		if (hideTimerRef.current != null) {
			clearTimeout(hideTimerRef.current)
			hideTimerRef.current = null
		}
	}

	useEffect(() => {
		dispatch({ type: 'reset' })
		shownAtRef.current = null
		clearHideTimer()

		if (!dialogId) {
			return
		}

		const handleEvent = (ev: AgentActivity) => {
			const now = Date.now()
			switch (ev.kind) {
				case 'tools_start':
					clearHideTimer()
					if (shownAtRef.current == null) {
						shownAtRef.current = now
					}
					dispatch({ type: 'tools_start', now })
					break
				case 'tools_end': {
					dispatch({ type: 'tools_end', now })
					const shownAt = shownAtRef.current ?? now
					clearHideTimer()
					hideTimerRef.current = setTimeout(() => {
						shownAtRef.current = null
						dispatch({ type: 'hide' })
						hideTimerRef.current = null
					}, hideDelay(shownAt, now))
					break
				}
				case 'turn_end':
					clearHideTimer()
					shownAtRef.current = null
					dispatch({ type: 'reset' })
					break
				case 'agent_result':
					clearHideTimer()
					shownAtRef.current = null
					dispatch({ type: 'reset' })
					onAgentResultRef.current?.()
					break
				// An action sub-agent posts its result into this dialog without
				// being part of a turn here, so the indicator state is left
				// alone: resetting it would blank the tool chip of whatever the
				// operator is running right now.
				case 'action_exec_done':
				case 'action_exec_failed':
					onActionResultRef.current?.()
					break
			}
		}

		const close = openDialogActivity(dialogId, handleEvent)

		return () => {
			close()
			clearHideTimer()
		}
	}, [dialogId])

	return state.visible
}
