import { useMemo } from 'react'
import { Save, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { buildContent, parseFrontmatter } from '@/lib/frontmatter'

interface FrontmatterEditorProps {
	fileName: string
	content: string
	isDirty: boolean
	loading: boolean
	saving: boolean
	onChange: (content: string) => void
	onSave: () => void
	onDelete: () => void
}

export function FrontmatterEditor({
	fileName,
	content,
	isDirty,
	loading,
	saving,
	onChange,
	onSave,
	onDelete,
}: FrontmatterEditorProps) {
	const fields = useMemo(() => parseFrontmatter(content), [content])

	function updateField(key: 'name' | 'description' | 'body', value: string) {
		onChange(buildContent({ ...fields, [key]: value }))
	}

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
						<Textarea
							id={`${fileName}-description`}
							value={fields.description}
							onChange={(e) => updateField('description', e.target.value)}
							placeholder="Short description"
							rows={2}
							disabled={saving}
						/>
					</div>

					<div className="flex min-h-0 flex-1 flex-col gap-2">
						<Label htmlFor={`${fileName}-body`}>Body</Label>
						<Textarea
							id={`${fileName}-body`}
							value={fields.body}
							onChange={(e) => updateField('body', e.target.value)}
							spellCheck={false}
							disabled={saving}
							className="min-h-0 flex-1 resize-none whitespace-pre font-mono"
							placeholder="Markdown content..."
						/>
					</div>
				</div>
			)}
		</div>
	)
}
