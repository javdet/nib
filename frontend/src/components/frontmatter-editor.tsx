import { useMemo } from 'react'
import { Save, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { cn } from '@/lib/utils'
import {
	buildContent,
	parseFrontmatter,
	type SkillCategory,
} from '@/lib/frontmatter'

interface FrontmatterEditorProps {
	fileName: string
	content: string
	isDirty: boolean
	loading: boolean
	saving: boolean
	showCategory?: boolean
	onChange: (content: string) => void
	onSave: () => void
	onDelete: () => void
}

const textareaClasses = cn(
	'w-full resize-y rounded-md border border-input bg-transparent px-3 py-2',
	'text-sm shadow-sm placeholder:text-muted-foreground',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
)

const bodyTextareaClasses = cn(
	textareaClasses,
	'min-h-0 flex-1 font-mono whitespace-pre resize-none',
)

export function FrontmatterEditor({
	fileName,
	content,
	isDirty,
	loading,
	saving,
	showCategory = false,
	onChange,
	onSave,
	onDelete,
}: FrontmatterEditorProps) {
	const fields = useMemo(() => parseFrontmatter(content), [content])

	function updateField(
		key: 'name' | 'description' | 'body' | 'category',
		value: string | SkillCategory | undefined,
	) {
		onChange(buildContent({ ...fields, [key]: value }))
	}

	const category: SkillCategory = fields.category ?? 'searchable'

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-3">
			<div className="flex flex-wrap items-center gap-2">
				<h3 className="font-mono text-sm font-medium">{fileName}.md</h3>
				{isDirty && (
					<span className="text-xs text-muted-foreground">Unsaved changes</span>
				)}
				<div className="ml-auto flex flex-wrap gap-2">
					<Button
						size="sm"
						onClick={onSave}
						disabled={loading || saving || !isDirty}
					>
						<Save className="mr-2 h-4 w-4" />
						{saving ? 'Saving...' : 'Save'}
					</Button>
					<Button
						size="sm"
						variant="destructive"
						onClick={onDelete}
						disabled={loading || saving}
					>
						<Trash2 className="mr-2 h-4 w-4" />
						Delete
					</Button>
				</div>
			</div>

			{loading ? (
				<p className="py-8 text-center text-sm text-muted-foreground">
					Loading...
				</p>
			) : (
				<div className="flex min-h-0 flex-1 flex-col gap-4">
					<div className="space-y-2">
						<Label htmlFor={`${fileName}-name`}>Name</Label>
						<Input
							id={`${fileName}-name`}
							value={fields.name}
							onChange={(e) => updateField('name', e.target.value)}
							placeholder="display name"
							disabled={saving}
						/>
					</div>

					<div className="space-y-2">
						<Label htmlFor={`${fileName}-description`}>Description</Label>
						<textarea
							id={`${fileName}-description`}
							value={fields.description}
							onChange={(e) => updateField('description', e.target.value)}
							placeholder="Short description"
							rows={2}
							disabled={saving}
							className={textareaClasses}
						/>
					</div>

					{showCategory && (
						<div className="space-y-2">
							<Label>Category</Label>
							<div className="inline-flex rounded-md border p-1">
								<Button
									type="button"
									size="sm"
									variant={
										category === 'searchable' ? 'secondary' : 'ghost'
									}
									onClick={() => updateField('category', undefined)}
									disabled={saving}
								>
									Searchable
								</Button>
								<Button
									type="button"
									size="sm"
									variant={
										category === 'included' ? 'secondary' : 'ghost'
									}
									onClick={() => updateField('category', 'included')}
									disabled={saving}
								>
									Included
								</Button>
							</div>
							<p className="text-xs text-muted-foreground">
								Included skills are listed in the discuss-mode system
								prompt. Searchable skills are discoverable later via
								search.
							</p>
						</div>
					)}

					<div className="flex min-h-0 flex-1 flex-col gap-2">
						<Label htmlFor={`${fileName}-body`}>Body</Label>
						<textarea
							id={`${fileName}-body`}
							value={fields.body}
							onChange={(e) => updateField('body', e.target.value)}
							spellCheck={false}
							disabled={saving}
							className={bodyTextareaClasses}
							placeholder="Markdown content..."
						/>
					</div>
				</div>
			)}
		</div>
	)
}
