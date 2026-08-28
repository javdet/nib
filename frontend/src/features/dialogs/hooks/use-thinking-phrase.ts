import { useEffect, useRef, useState } from 'react'
import { thinkingPhrase } from '@/features/dialogs/lib/thinking-phrases'

export function useThinkingPhrase(active: boolean): string {
	const [turn, setTurn] = useState(0)
	const wasActiveRef = useRef(false)

	useEffect(() => {
		if (!active && wasActiveRef.current) {
			setTurn((prev) => prev + 1)
		}
		wasActiveRef.current = active
	}, [active])

	return thinkingPhrase(turn)
}
