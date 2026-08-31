import { useState, useEffect, useCallback } from 'react'
import { Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Textarea } from '@/components/ui/textarea'
import { extractErrorMessage } from '@/lib/api-client'
import { getDiscussPrompt, updateDiscussPrompt } from '../api/knowledge'

export function DiscussPromptCard() {
	const [content, setContent] = useState('')
	const [savedContent, setSavedContent] = useState('')
	const [loading, setLoading] = useState(true)
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)

	const isDirty = content !== savedContent

	useEffect(() => {
		let cancelled = false
		setLoading(true)
		setError(null)

		getDiscussPrompt()
			.then((prompt) => {
				if (cancelled) return
				setContent(prompt.content)
				setSavedContent(prompt.content)
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

	const handleSave = useCallback(async () => {
		if (!isDirty) return
		setError(null)
		setSaving(true)
		try {
			await updateDiscussPrompt(content)
			setSavedContent(content)
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setSaving(false)
		}
	}, [content, isDirty])

	return (
		<Card>
			<CardHeader className="flex flex-row items-center justify-between space-y-0">
				<div>
					<CardTitle>Discuss prompt</CardTitle>
					<p className="mt-1 text-sm text-muted-foreground">
						System prompt for discuss-mode chats. Supports template variables
						like {'{{ .global.CompanyName }}'}.
					</p>
				</div>
				<div className="flex items-center gap-2">
					{isDirty && (
						<span className="text-xs text-muted-foreground">
							Unsaved changes
						</span>
					)}
					<Button
						size="sm"
						onClick={() => void handleSave()}
						disabled={loading || saving || !isDirty}
					>
						<Save className="mr-2 h-4 w-4" />
						{saving ? 'Saving...' : 'Save'}
					</Button>
				</div>
			</CardHeader>
			<CardContent>
				{error && (
					<div className="mb-4 rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
						{error}
					</div>
				)}
				{loading ? (
					<p className="py-8 text-center text-sm text-muted-foreground">
						Loading prompt...
					</p>
				) : (
					<Textarea
						value={content}
						onChange={(e) => setContent(e.target.value)}
						spellCheck={false}
						disabled={saving}
						className="min-h-[240px] whitespace-pre font-mono"
						placeholder="Write the discuss system prompt in markdown..."
					/>
				)}
			</CardContent>
		</Card>
	)
}
