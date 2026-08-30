import { useCallback, useEffect, useState } from 'react'
import { Check, Copy, Download } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogFooter,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { downloadTextFile } from '@/lib/download'
import { extractErrorMessage } from '@/lib/api-client'
import { type KnowledgeDocument, getDocument } from '../api/knowledge'

interface ViewDocumentDialogProps {
	collection: string
	open: boolean
	onOpenChange: (open: boolean) => void
}

export function ViewDocumentDialog({
	collection,
	open,
	onOpenChange,
}: ViewDocumentDialogProps) {
	const [document, setDocument] = useState<KnowledgeDocument | null>(null)
	const [loading, setLoading] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [copied, setCopied] = useState(false)

	// Refetched on every open so the dialog reflects the last upload rather than
	// a body cached from before it.
	useEffect(() => {
		if (!open) return

		let cancelled = false
		setLoading(true)
		setError(null)
		setDocument(null)
		setCopied(false)

		getDocument(collection)
			.then((doc) => {
				if (!cancelled) setDocument(doc)
			})
			.catch((err) => {
				if (!cancelled) setError(extractErrorMessage(err))
			})
			.finally(() => {
				if (!cancelled) setLoading(false)
			})

		return () => {
			cancelled = true
		}
	}, [collection, open])

	const handleCopy = useCallback(async () => {
		if (!document) return
		try {
			await navigator.clipboard.writeText(document.content)
			setCopied(true)
		} catch (err) {
			setError(extractErrorMessage(err))
		}
	}, [document])

	const handleDownload = useCallback(() => {
		if (!document) return
		downloadTextFile(
			document.filename ?? `${document.collection}.md`,
			document.content,
		)
	}, [document])

	const isTemplate = document?.source === 'template'

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="flex max-h-[85vh] max-w-3xl flex-col">
				<DialogHeader>
					<DialogTitle className="flex items-center gap-2">
						Knowledge base
						<Badge variant="secondary">{collection}</Badge>
						{document && (
							<Badge variant={isTemplate ? 'outline' : 'default'}>
								{document.source}
							</Badge>
						)}
					</DialogTitle>
					<DialogDescription>
						{isTemplate
							? 'Nothing has been uploaded for this collection yet. This is the built-in template — fill it in and upload it to index it.'
							: describeUploaded(document)}
					</DialogDescription>
				</DialogHeader>

				{error && (
					<div className="rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive">
						{error}
					</div>
				)}

				{loading && (
					<p className="py-8 text-center text-sm text-muted-foreground">
						Loading document...
					</p>
				)}

				{document && (
					// The raw source, not rendered markdown: this is exactly what was
					// chunked and indexed, and rendering would swallow the HTML comments
					// the template carries its guidance in.
					<pre className="min-h-0 flex-1 overflow-auto rounded-md border bg-muted/40 p-4 font-mono text-xs leading-relaxed whitespace-pre-wrap">
						{document.content}
					</pre>
				)}

				<DialogFooter>
					<Button
						variant="outline"
						onClick={() => void handleCopy()}
						disabled={!document}
					>
						{copied ? (
							<>
								<Check className="h-4 w-4" />
								Copied
							</>
						) : (
							<>
								<Copy className="h-4 w-4" />
								Copy
							</>
						)}
					</Button>
					<Button variant="outline" onClick={handleDownload} disabled={!document}>
						<Download className="h-4 w-4" />
						Download
					</Button>
					<Button onClick={() => onOpenChange(false)}>Close</Button>
				</DialogFooter>
			</DialogContent>
		</Dialog>
	)
}

function describeUploaded(document: KnowledgeDocument | null): string {
	if (!document) return 'The document currently indexed in this collection.'

	const parts = [document.filename ?? 'uploaded document']
	if (document.updatedAt) {
		parts.push(`uploaded ${new Date(document.updatedAt).toLocaleString()}`)
	}
	return parts.join(' — ')
}
