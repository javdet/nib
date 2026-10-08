import { useState, type ReactNode } from 'react'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { IconButton } from '@/components/ui/icon-button'
import { Input } from '@/components/ui/input'
import { Select } from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { cn } from '@/lib/utils'
import type { PostmortemItemFields } from '../api/incidents'
import { fromLocalInputValue, toLocalInputValue } from '../lib/incident-time'
import { OperatorLockBadge } from './operator-lock-badge'

export interface ListFieldSpec {
	key: keyof PostmortemItemFields
	label: string
	placeholder?: string
	kind?: 'text' | 'textarea' | 'datetime' | 'select'
	options?: { value: string; label: string }[]
	required?: boolean
	/** Grid columns the field spans in the edit form, out of 6. */
	span?: 1 | 2 | 3 | 4 | 6
}

interface ListItem {
	id: string
	operator?: boolean
}

interface EditableListCardProps<T extends ListItem> {
	title: string
	description?: string
	items: T[]
	fields: ListFieldSpec[]
	toFields: (item: T) => PostmortemItemFields
	renderItem: (item: T) => ReactNode
	emptyText: string
	addLabel: string
	onAdd: (fields: PostmortemItemFields) => Promise<void>
	onUpdate: (id: string, fields: PostmortemItemFields) => Promise<void>
	onDelete: (id: string) => Promise<void>
	/** Rendered before each row's content, such as a selection checkbox. */
	leading?: (item: T) => ReactNode
	/** Items that cannot be deleted, such as an action already in a plan. */
	canDelete?: (item: T) => boolean
	headerActions?: ReactNode
	footer?: ReactNode
}

const SPAN_CLASSES: Record<number, string> = {
	1: 'sm:col-span-1',
	2: 'sm:col-span-2',
	3: 'sm:col-span-3',
	4: 'sm:col-span-4',
	6: 'sm:col-span-6',
}

function ItemForm({
	fields,
	initial,
	onSubmit,
	onCancel,
	submitLabel,
}: {
	fields: ListFieldSpec[]
	initial: PostmortemItemFields
	onSubmit: (fields: PostmortemItemFields) => Promise<void>
	onCancel: () => void
	submitLabel: string
}) {
	const [values, setValues] = useState<PostmortemItemFields>(() => {
		const out: PostmortemItemFields = { ...initial }
		for (const field of fields) {
			if (field.kind === 'datetime') {
				out[field.key] = toLocalInputValue(initial[field.key] ?? '')
			}
		}
		return out
	})
	const [saving, setSaving] = useState(false)
	const [error, setError] = useState<string | null>(null)

	const missing = fields.some(
		(field) => field.required && !(values[field.key] ?? '').trim(),
	)

	const handleSubmit = async () => {
		const out: PostmortemItemFields = {}
		for (const field of fields) {
			const value = values[field.key] ?? ''
			out[field.key] = field.kind === 'datetime' ? fromLocalInputValue(value) : value.trim()
		}
		setSaving(true)
		setError(null)
		try {
			await onSubmit(out)
		} catch (err) {
			setError(err instanceof Error ? err.message : 'Failed to save')
			setSaving(false)
		}
	}

	const set = (key: keyof PostmortemItemFields, value: string) =>
		setValues((prev) => ({ ...prev, [key]: value }))

	return (
		<div
			className="space-y-3 rounded-md border border-border/60 bg-muted/20 p-3"
			onKeyDown={(e) => {
				if (e.key === 'Escape') {
					e.preventDefault()
					onCancel()
				}
				if (e.key === 'Enter' && (e.metaKey || e.ctrlKey) && !missing) {
					e.preventDefault()
					void handleSubmit()
				}
			}}
		>
			<div className="grid grid-cols-1 gap-3 sm:grid-cols-6">
				{fields.map((field) => {
					const value = values[field.key] ?? ''
					const id = `field-${field.key}`
					return (
						<label
							key={field.key}
							htmlFor={id}
							className={cn('space-y-1 text-xs font-medium', SPAN_CLASSES[field.span ?? 6])}
						>
							<span className="text-muted-foreground">{field.label}</span>
							{field.kind === 'textarea' ? (
								<Textarea
									id={id}
									value={value}
									placeholder={field.placeholder}
									onChange={(e) => set(field.key, e.target.value)}
									disabled={saving}
									className="min-h-[72px] text-sm font-normal"
								/>
							) : field.kind === 'select' ? (
								<Select
									id={id}
									value={value}
									onChange={(e) => set(field.key, e.target.value)}
									disabled={saving}
									className="font-normal"
								>
									{field.options?.map((option) => (
										<option key={option.value} value={option.value}>
											{option.label}
										</option>
									))}
								</Select>
							) : (
								<Input
									id={id}
									type={field.kind === 'datetime' ? 'datetime-local' : 'text'}
									value={value}
									placeholder={field.placeholder}
									onChange={(e) => set(field.key, e.target.value)}
									disabled={saving}
									className="font-normal"
								/>
							)}
						</label>
					)
				})}
			</div>
			{error && <p className="text-sm text-destructive">{error}</p>}
			<div className="flex gap-2">
				<Button
					type="button"
					size="sm"
					onClick={() => void handleSubmit()}
					disabled={saving || missing}
				>
					{saving ? 'Saving...' : submitLabel}
				</Button>
				<Button type="button" variant="outline" size="sm" onClick={onCancel} disabled={saving}>
					Cancel
				</Button>
			</div>
		</div>
	)
}

export function EditableListCard<T extends ListItem>({
	title,
	description,
	items,
	fields,
	toFields,
	renderItem,
	emptyText,
	addLabel,
	onAdd,
	onUpdate,
	onDelete,
	leading,
	canDelete,
	headerActions,
	footer,
}: EditableListCardProps<T>) {
	const [editingId, setEditingId] = useState<string | null>(null)
	const [adding, setAdding] = useState(false)
	const [deletingId, setDeletingId] = useState<string | null>(null)
	const [error, setError] = useState<string | null>(null)

	const handleDelete = async (id: string) => {
		setDeletingId(id)
		setError(null)
		try {
			await onDelete(id)
		} catch (err) {
			setError(err instanceof Error ? err.message : 'Failed to delete')
		} finally {
			setDeletingId(null)
		}
	}

	const defaults: PostmortemItemFields = {}
	for (const field of fields) {
		if (field.kind === 'select' && field.options?.[0]) {
			defaults[field.key] = field.options[0].value
		}
	}

	return (
		<Card>
			<CardHeader>
				<div className="flex items-center gap-2">
					<div className="min-w-0">
						<CardTitle>{title}</CardTitle>
						{description && (
							<p className="mt-1 text-xs text-muted-foreground">{description}</p>
						)}
					</div>
					<div className="ml-auto flex items-center gap-1">
						{headerActions}
						{!adding && (
							<IconButton
								type="button"
								variant="ghost"
								className="h-8 w-8 text-muted-foreground"
								onClick={() => {
									setAdding(true)
									setEditingId(null)
								}}
								tooltip={addLabel}
							>
								<Plus className="h-4 w-4" />
							</IconButton>
						)}
					</div>
				</div>
			</CardHeader>
			<CardContent className="space-y-2">
				{items.length === 0 && !adding && (
					<p className="text-sm text-muted-foreground">{emptyText}</p>
				)}
				{items.map((item) =>
					editingId === item.id ? (
						<ItemForm
							key={item.id}
							fields={fields}
							initial={toFields(item)}
							submitLabel="Save"
							onCancel={() => setEditingId(null)}
							onSubmit={async (values) => {
								await onUpdate(item.id, values)
								setEditingId(null)
							}}
						/>
					) : (
						<div
							key={item.id}
							className="group flex items-start gap-3 rounded-md px-2 py-1.5 hover:bg-muted/30"
						>
							{leading?.(item)}
							<div className="min-w-0 flex-1 text-sm">{renderItem(item)}</div>
							{item.operator && <OperatorLockBadge compact />}
							<div className="flex shrink-0 items-center opacity-60 transition-opacity group-hover:opacity-100">
								<IconButton
									type="button"
									variant="ghost"
									className="h-7 w-7 text-muted-foreground"
									onClick={() => {
										setEditingId(item.id)
										setAdding(false)
									}}
									tooltip="Edit"
								>
									<Pencil className="h-3.5 w-3.5" />
								</IconButton>
								{(canDelete?.(item) ?? true) && (
									<IconButton
										type="button"
										variant="ghost"
										className="h-7 w-7 text-muted-foreground hover:text-destructive"
										onClick={() => void handleDelete(item.id)}
										disabled={deletingId === item.id}
										tooltip="Delete"
									>
										<Trash2 className="h-3.5 w-3.5" />
									</IconButton>
								)}
							</div>
						</div>
					),
				)}
				{adding && (
					<ItemForm
						fields={fields}
						initial={defaults}
						submitLabel="Add"
						onCancel={() => setAdding(false)}
						onSubmit={async (values) => {
							await onAdd(values)
							setAdding(false)
						}}
					/>
				)}
				{error && <p className="text-sm text-destructive">{error}</p>}
				{footer}
			</CardContent>
		</Card>
	)
}
