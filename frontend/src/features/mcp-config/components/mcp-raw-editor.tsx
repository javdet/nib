import { useMemo } from 'react'
import { Save } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface MCPRawEditorProps {
	content: string
	isDirty: boolean
	loading: boolean
	saving: boolean
	onChange: (content: string) => void
	onSave: () => void
}

const textareaClasses = cn(
	'min-h-[24rem] w-full resize-y rounded-md border border-input bg-transparent px-3 py-2',
	'font-mono text-sm shadow-sm placeholder:text-muted-foreground',
	'focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring',
	'whitespace-pre',
)

function validateMCPJSON(text: string): string | null {
	const trimmed = text.trim()
	if (!trimmed) {
		return 'JSON cannot be empty'
	}
	let parsed: unknown
	try {
		parsed = JSON.parse(trimmed)
	} catch {
		return 'Invalid JSON syntax'
	}
	if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
		return 'Root value must be a JSON object'
	}
	const doc = parsed as Record<string, unknown>
	if (!('mcpServers' in doc)) {
		return 'Missing required "mcpServers" property'
	}
	if (
		doc.mcpServers === null ||
		typeof doc.mcpServers !== 'object' ||
		Array.isArray(doc.mcpServers)
	) {
		return '"mcpServers" must be an object'
	}
	return null
}

export function MCPRawEditor({
	content,
	isDirty,
	loading,
	saving,
	onChange,
	onSave,
}: MCPRawEditorProps) {
	const validationError = useMemo(() => {
		if (!content.trim()) return null
		return validateMCPJSON(content)
	}, [content])

	function handleSave() {
		if (validateMCPJSON(content)) return
		onSave()
	}

	return (
		<div className="flex min-h-0 flex-1 flex-col gap-3">
			<div className="flex flex-wrap items-center gap-2">
				<h3 className="text-sm font-medium">mcp.json</h3>
				{isDirty && (
					<span className="text-xs text-muted-foreground">
						Unsaved changes
					</span>
				)}
				<div className="ml-auto">
					<Button
						size="sm"
						onClick={handleSave}
						disabled={
							loading ||
							saving ||
							!isDirty ||
							!!validationError
						}
					>
						<Save className="mr-2 h-4 w-4" />
						{saving ? 'Saving...' : 'Save'}
					</Button>
				</div>
			</div>

			<p className="text-xs text-muted-foreground">
				Use <code className="font-mono">{'${SECRET_NAME}'}</code> in any
				value to reference a secret or environment variable. It is
				substituted when the server is contacted, so the token itself is
				never stored here.
			</p>

			{loading ? (
				<p className="py-8 text-center text-sm text-muted-foreground">
					Loading mcp.json...
				</p>
			) : (
				<>
					<textarea
						value={content}
						onChange={(e) => onChange(e.target.value)}
						spellCheck={false}
						disabled={saving}
						className={textareaClasses}
						placeholder='{"mcpServers": {}}'
					/>
					{validationError && (
						<p className="text-sm text-destructive">{validationError}</p>
					)}
				</>
			)}
		</div>
	)
}

export { validateMCPJSON }
