import { useState, useEffect, useCallback } from 'react'
import { extractErrorMessage } from '@/lib/api-client'

export interface MarkdownResourceAPI {
	list: () => Promise<{ names: string[] }>
	get: (name: string) => Promise<{ content: string }>
	create: (name: string, content: string) => Promise<void>
	update: (name: string, content: string) => Promise<void>
	delete: (name: string) => Promise<void>
}

export interface NamedMarkdownResource {
	names: string[]
	selectedName: string | null
	content: string
	isDirty: boolean
	listLoading: boolean
	contentLoading: boolean
	saving: boolean
	error: string | null
	selectName: (name: string) => void
	setContent: (content: string) => void
	save: () => Promise<void>
	remove: () => Promise<void>
	create: (name: string, content: string) => Promise<void>
	refreshList: () => Promise<{ names: string[] }>
}

export function useNamedMarkdownResource(
	api: MarkdownResourceAPI,
	resourceLabel: string,
): NamedMarkdownResource {
	const [names, setNames] = useState<string[]>([])
	const [selectedName, setSelectedName] = useState<string | null>(null)
	const [content, setContent] = useState('')
	const [savedContent, setSavedContent] = useState('')
	const [listLoading, setListLoading] = useState(true)
	const [contentLoading, setContentLoading] = useState(false)
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)

	const isDirty = selectedName !== null && content !== savedContent

	const refreshList = useCallback(async () => {
		const result = await api.list()
		setNames(result.names)
		return result
	}, [api])

	useEffect(() => {
		refreshList()
			.then((result) => {
				const initial = result.names[0] ?? null
				if (initial) {
					setSelectedName(initial)
				}
			})
			.catch(() => {
				setNames([])
			})
			.finally(() => setListLoading(false))
	}, [refreshList])

	useEffect(() => {
		if (!selectedName) {
			setContent('')
			setSavedContent('')
			return
		}

		let cancelled = false
		setContentLoading(true)
		setError(null)

		api
			.get(selectedName)
			.then((doc) => {
				if (cancelled) return
				setContent(doc.content)
				setSavedContent(doc.content)
			})
			.catch((err) => {
				if (cancelled) return
				setError(extractErrorMessage(err))
				setContent('')
				setSavedContent('')
			})
			.finally(() => {
				if (!cancelled) setContentLoading(false)
			})

		return () => {
			cancelled = true
		}
	}, [api, selectedName])

	const selectName = useCallback(
		(name: string) => {
			if (name === selectedName) return
			if (isDirty) {
				const leave = window.confirm(
					`You have unsaved changes. Discard them and switch ${resourceLabel}?`,
				)
				if (!leave) return
			}
			setSelectedName(name)
		},
		[isDirty, resourceLabel, selectedName],
	)

	const save = useCallback(async () => {
		if (!selectedName || !isDirty) return
		setError(null)
		setSaving(true)
		try {
			await api.update(selectedName, content)
			setSavedContent(content)
			await refreshList()
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}, [api, selectedName, content, isDirty, refreshList])

	const remove = useCallback(async () => {
		if (!selectedName) return
		const confirmed = window.confirm(
			`Delete "${selectedName}.md"? This cannot be undone.`,
		)
		if (!confirmed) return

		setError(null)
		setSaving(true)
		try {
			await api.delete(selectedName)
			const result = await refreshList()
			setSelectedName(result.names[0] ?? null)
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}, [api, selectedName, refreshList])

	const create = useCallback(
		async (name: string, docContent: string) => {
			setError(null)
			setSaving(true)
			try {
				await api.create(name, docContent)
				await refreshList()
				setSelectedName(name)
			} catch (err) {
				setError(extractErrorMessage(err))
				throw err
			} finally {
				setSaving(false)
			}
		},
		[api, refreshList],
	)

	return {
		names,
		selectedName,
		content,
		isDirty,
		listLoading,
		contentLoading,
		saving,
		error,
		selectName,
		setContent,
		save,
		remove,
		create,
		refreshList,
	}
}
