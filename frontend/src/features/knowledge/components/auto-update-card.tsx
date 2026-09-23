import { useState, useEffect, useCallback, useId } from 'react'
import { Checkbox } from '@/components/ui/checkbox'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { extractErrorMessage } from '@/lib/api-client'
import { getKnowledgeSettings, updateKnowledgeSettings } from '../api/knowledge'

export function AutoUpdateCard() {
	const autoUpdateId = useId()
	const [autoUpdate, setAutoUpdate] = useState(false)
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		let cancelled = false
		setLoading(true)
		setError(null)

		getKnowledgeSettings()
			.then((settings) => {
				if (cancelled) return
				setAutoUpdate(settings.autoUpdate)
			})
			.catch((err) => {
				if (cancelled) return
				setError(extractErrorMessage(err))
			})
			.finally(() => {
				if (!cancelled) setLoading(false)
			})

		return () => {
			cancelled = true
		}
	}, [])

	// The checkbox is the whole form, so it saves on change rather than behind a
	// button. The previous value is put back when the write fails, so what is
	// shown is always what the server holds.
	const handleChange = useCallback(
		async (next: boolean) => {
			const previous = autoUpdate
			setAutoUpdate(next)
			setSaving(true)
			setError(null)
			try {
				const settings = await updateKnowledgeSettings({ autoUpdate: next })
				setAutoUpdate(settings.autoUpdate)
			} catch (err) {
				setAutoUpdate(previous)
				setError(extractErrorMessage(err))
			} finally {
				setSaving(false)
			}
		},
		[autoUpdate],
	)

	return (
		<Card>
			<CardHeader>
				<CardTitle>Automatic updates</CardTitle>
			</CardHeader>
			<CardContent className="space-y-3">
				{error && (
					<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
						{error}
					</div>
				)}

				<div className="flex items-start gap-2">
					<Checkbox
						id={autoUpdateId}
						checked={autoUpdate}
						onCheckedChange={(value) => void handleChange(value === true)}
						disabled={loading || saving}
					/>
					<div className="space-y-1">
						<Label htmlFor={autoUpdateId} className="cursor-pointer">
							Update the knowledge base when a plan is finished
						</Label>
						<p className="text-sm text-muted-foreground">
							Finishing a plan starts a background agent that merges what the
							work established into the collection named after the plan's
							project, leaving every other fact in it untouched. A plan with no
							project selected is skipped.
						</p>
					</div>
				</div>
			</CardContent>
		</Card>
	)
}
