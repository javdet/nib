import { useState, useEffect, useCallback } from 'react'
import {
	listDialogsPaged,
	listPinnedDialogs,
	type Dialog,
	type PagedDialogs,
} from '@/features/dialogs/api/dialogs'

export interface UsePagedDialogsOptions {
	pageSize: number
	mode?: string
	search?: string
	withPinned?: boolean
	refreshKey?: number
}

export function usePagedDialogs({
	pageSize,
	mode,
	search,
	withPinned = true,
	refreshKey = 0,
}: UsePagedDialogsOptions) {
	const [dialogs, setDialogs] = useState<Dialog[]>([])
	const [pinnedDialogs, setPinnedDialogs] = useState<Dialog[]>([])
	const [page, setPage] = useState(1)
	const [total, setTotal] = useState(0)
	const [loading, setLoading] = useState(true)

	const loadDialogs = useCallback(() => {
		setLoading(true)
		const activeQuery = search?.trim() ?? ''

		if (activeQuery) {
			listDialogsPaged(page, pageSize, undefined, mode, activeQuery)
				.then(({ items, total: totalCount }) => {
					setPinnedDialogs([])
					setDialogs(items)
					setTotal(totalCount)
					if (items.length === 0 && page > 1 && totalCount > 0) {
						setPage(Math.ceil(totalCount / pageSize))
					}
				})
				.catch(() => {
					setPinnedDialogs([])
					setDialogs([])
					setTotal(0)
				})
				.finally(() => setLoading(false))
			return
		}

		const requests: Promise<[Dialog[], PagedDialogs]> = withPinned
			? Promise.all([
					listPinnedDialogs(),
					listDialogsPaged(page, pageSize, undefined, mode),
				]).then(([pinned, paged]) => [pinned, paged])
			: listDialogsPaged(page, pageSize, undefined, mode).then((paged) => [
					[] as Dialog[],
					paged,
				])

		requests
			.then(([pinned, { items, total: totalCount }]) => {
				setPinnedDialogs(pinned)
				setDialogs(items)
				setTotal(totalCount)
				if (items.length === 0 && page > 1 && totalCount > 0) {
					setPage(Math.ceil(totalCount / pageSize))
				}
			})
			.catch(() => {
				setPinnedDialogs([])
				setDialogs([])
				setTotal(0)
			})
			.finally(() => setLoading(false))
	}, [page, pageSize, mode, search, withPinned])

	useEffect(() => {
		loadDialogs()
	}, [loadDialogs, refreshKey])

	return {
		dialogs,
		pinnedDialogs,
		page,
		total,
		loading,
		setPage,
		reload: loadDialogs,
	}
}
