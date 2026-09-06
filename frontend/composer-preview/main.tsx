import { useRef, useState } from 'react'
import { createRoot } from 'react-dom/client'
import { Textarea } from '../src/components/ui/textarea'
import { useAutosizeTextarea } from '../src/hooks/use-autosize-textarea'
import './preview.css'

function Panel({ initial, label }: { initial: string; label: string }) {
	const [draft, setDraft] = useState(initial)
	const panelRef = useRef<HTMLDivElement>(null)
	const textareaRef = useRef<HTMLTextAreaElement>(null)
	useAutosizeTextarea(textareaRef, panelRef, draft)
	return (
		<div className="flex flex-col gap-1">
			<span className="text-xs text-muted-foreground">{label}</span>
			<div
				ref={panelRef}
				className="flex h-[500px] w-[360px] min-h-0 min-w-0 flex-col rounded-md border"
			>
				<div className="min-h-0 flex-1 overflow-y-auto p-4 text-sm text-muted-foreground">
					messages area
				</div>
				<div className="relative shrink-0 border-t p-3">
					<div className="flex min-w-0 gap-2">
						<Textarea
							ref={textareaRef}
							value={draft}
							onChange={(e) => setDraft(e.target.value)}
							placeholder="Message…"
							rows={2}
							className="min-h-[72px] min-w-0 resize-none overflow-hidden"
						/>
						<div className="flex shrink-0 flex-col gap-2 self-end">
							<div className="h-9 w-9 rounded-md border" />
							<div className="h-9 w-9 rounded-md border" />
						</div>
					</div>
				</div>
			</div>
		</div>
	)
}

const short = 'Deploy the staging cluster.'
const mid = Array.from({ length: 6 }, (_, i) => `Line ${i + 1} of the draft message`).join('\n')
const long = Array.from({ length: 40 }, (_, i) => `Line ${i + 1} of a very long draft message that keeps going`).join('\n')

createRoot(document.getElementById('root')!).render(
	<div className="flex gap-6 bg-background p-6">
		<Panel initial="" label="empty" />
		<Panel initial={short} label="one line" />
		<Panel initial={mid} label="six lines" />
		<Panel initial={long} label="overflowing (capped)" />
	</div>,
)
