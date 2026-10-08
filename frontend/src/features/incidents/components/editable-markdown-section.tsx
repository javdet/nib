import { useRef, useState, type ReactNode } from 'react'
import { ChevronDown, ChevronRight, Pencil } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { IconButton } from '@/components/ui/icon-button'
import { Textarea } from '@/components/ui/textarea'
import { MarkdownMessage } from '@/components/markdown-message'
import { OperatorLockBadge } from './operator-lock-badge'

interface EditableMarkdownSectionProps {
	title: string
	content: string
	emptyText: string
	/** The operator owns the field; agents cannot overwrite it. */
	locked?: boolean
	onSave: (content: string) => Promise<void>
	onUnlock?: () => Promise<void>
	defaultExpanded?: boolean
	onDialogLink?: (dialogId: string) => void
	/** Extra controls rendered in the header, right-aligned. */
	actions?: ReactNode
}

export function EditableMarkdownSection({
	title,
	content,
	emptyText,
	locked = false,
	onSave,
	onUnlock,
	defaultExpanded = true,
	onDialogLink,
	actions,
}: EditableMarkdownSectionProps) {
	const [expanded, setExpanded] = useState(defaultExpanded)
	const [editing, setEditing] = useState(false)
	const [draft, setDraft] = useState('')
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const textareaRef = useRef<HTMLTextAreaElement>(null)

	const handleStartEdit = () => {
		setDraft(content)
		setError(null)
		setEditing(true)
		setExpanded(true)
		requestAnimationFrame(() => textareaRef.current?.focus())
	}

	const handleCancel = () => {
		setEditing(false)
		setError(null)
	}

	const handleSave = async () => {
		setSaving(true)
		setError(null)
		try {
			await onSave(draft.trim())
			setEditing(false)
		} catch (err) {
			setError(err instanceof Error ? err.message : 'Failed to save')
		} finally {
			setSaving(false)
		}
	}

	const toggle = () => setExpanded((prev) => !prev)

	return (
		<Card>
			<CardHeader>
				<div
					role="button"
					tabIndex={0}
					aria-expanded={expanded}
					className="flex cursor-pointer items-center gap-2 rounded-sm outline-none focus-visible:ring-2 focus-visible:ring-ring"
					onClick={toggle}
					onKeyDown={(e) => {
						if (e.key === 'Enter' || e.key === ' ') {
							e.preventDefault()
							toggle()
						}
					}}
				>
					{expanded ? (
						<ChevronDown className="h-4 w-4 shrink-0 text-muted-foreground" />
					) : (
						<ChevronRight className="h-4 w-4 shrink-0 text-muted-foreground" />
					)}
					<CardTitle>{title}</CardTitle>
					{locked && <OperatorLockBadge onUnlock={onUnlock} />}
					<div className="ml-auto flex items-center gap-1">
						{actions}
						{!editing && (
							<IconButton
								type="button"
								variant="ghost"
								className="h-8 w-8 shrink-0 text-muted-foreground"
								onMouseDown={(e) => e.preventDefault()}
								onClick={(e) => {
									e.stopPropagation()
									handleStartEdit()
								}}
								tooltip={`Edit ${title.toLowerCase()}`}
							>
								<Pencil className="h-4 w-4" />
							</IconButton>
						)}
					</div>
				</div>
			</CardHeader>
			{expanded && (
				<CardContent>
					{editing ? (
						<div className="space-y-3">
							<Textarea
								ref={textareaRef}
								value={draft}
								onChange={(e) => setDraft(e.target.value)}
								onKeyDown={(e) => {
									if (e.key === 'Escape') {
										e.preventDefault()
										handleCancel()
									}
									if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
										e.preventDefault()
										void handleSave()
									}
								}}
								disabled={saving}
								className="min-h-[120px] whitespace-pre-wrap"
								aria-label={title}
							/>
							{error && <p className="text-sm text-destructive">{error}</p>}
							<div className="flex gap-2">
								<Button
									type="button"
									size="sm"
									onClick={() => void handleSave()}
									disabled={saving}
								>
									{saving ? 'Saving...' : 'Save'}
								</Button>
								<Button
									type="button"
									variant="outline"
									size="sm"
									onClick={handleCancel}
									disabled={saving}
								>
									Cancel
								</Button>
							</div>
						</div>
					) : content ? (
						<div className="text-sm">
							<MarkdownMessage content={content} onDialogLink={onDialogLink} />
						</div>
					) : (
						<p className="text-sm text-muted-foreground">{emptyText}</p>
					)}
				</CardContent>
			)}
		</Card>
	)
}
