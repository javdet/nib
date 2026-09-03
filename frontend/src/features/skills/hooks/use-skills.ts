import { useCallback, useEffect, useRef, useState } from 'react'
import { listSkills, type SkillMeta } from '../api/skills'

// How long a loaded list is trusted before opening the menu revalidates it. Skills
// can be created or renamed on the skills page while the chat panel stays mounted.
const STALE_MS = 60_000

/**
 * The skill catalog for the slash menu.
 *
 * Failures are swallowed on purpose: the list only feeds an autocomplete, and
 * surfacing an error here would paint the chat's error banner over something the
 * operator never asked for. An empty list simply means the menu never opens.
 */
export function useSkills() {
	const [skills, setSkills] = useState<SkillMeta[]>([])
	const loadedAtRef = useRef(0)
	const cancelledRef = useRef(false)

	const reload = useCallback(async () => {
		try {
			const result = await listSkills()
			if (cancelledRef.current) return
			setSkills(result.skills)
			loadedAtRef.current = Date.now()
		} catch {
			// Keep whatever was loaded before; the menu degrades to nothing.
		}
	}, [])

	useEffect(() => {
		cancelledRef.current = false
		void reload()
		return () => {
			cancelledRef.current = true
		}
	}, [reload])

	const isStale = useCallback(
		() => Date.now() - loadedAtRef.current > STALE_MS,
		[],
	)

	return { skills, reload, isStale }
}
