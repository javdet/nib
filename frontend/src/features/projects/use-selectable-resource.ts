import { useCallback, useEffect, useState } from 'react'

const ANY_NAME = 'any'

export interface SelectableItem {
	id: string
	name: string
}

export function useSelectableResource<T extends SelectableItem>(
	storageKey: string,
	items: T[],
) {
	const [selectedId, setSelectedId] = useState<string | null>(
		() => localStorage.getItem(storageKey),
	)

	useEffect(() => {
		if (items.length === 0) return

		const current = items.find((item) => item.id === selectedId)
		if (current) return

		const fallback = items.find((item) => item.name === ANY_NAME)
		if (!fallback) return

		setSelectedId(fallback.id)
		localStorage.setItem(storageKey, fallback.id)
	}, [items, selectedId, storageKey])

	const selected = items.find((item) => item.id === selectedId) ?? null

	const select = useCallback(
		(item: T) => {
			setSelectedId(item.id)
			localStorage.setItem(storageKey, item.id)
		},
		[storageKey],
	)

	return { selected, selectedId, select }
}
