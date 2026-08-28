import { useRef, useState } from 'react'
import { FileUp, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import {
	type KnowledgeCollection,
	isValidKnowledgeCollectionName,
} from '../api/knowledge'
import { CollectionSelect } from './collection-select'

interface UploadDocumentCardProps {
	collections: KnowledgeCollection[]
	collection: string
	onCollectionChange: (name: string) => void
	uploading: boolean
	onUpload: (file: File) => void
}

export function UploadDocumentCard({
	collections,
	collection,
	onCollectionChange,
	uploading,
	onUpload,
}: UploadDocumentCardProps) {
	const inputRef = useRef<HTMLInputElement>(null)
	const [selectedName, setSelectedName] = useState<string | null>(null)

	const trimmedCollection = collection.trim()
	const selectedCollection = collections.find(
		(item) => item.name === trimmedCollection,
	)
	const isNewCollection =
		trimmedCollection.length > 0 && selectedCollection === undefined
	const isCollectionValid = isValidKnowledgeCollectionName(collection)

	function handleFileChange(e: React.ChangeEvent<HTMLInputElement>) {
		const file = e.target.files?.[0]
		setSelectedName(file?.name ?? null)
	}

	function handleUpload() {
		const file = inputRef.current?.files?.[0]
		if (!file || !isCollectionValid) return
		onUpload(file)
	}

	const displayCollection = trimmedCollection || 'default'

	return (
		<Card>
			<CardHeader>
				<CardTitle>Document upload</CardTitle>
			</CardHeader>
			<CardContent className="space-y-4">
				<div className="space-y-2">
					<Label htmlFor="kb-collection">Collection</Label>
					<CollectionSelect
						collections={collections}
						value={collection}
						onChange={onCollectionChange}
						disabled={uploading}
					/>
				</div>

				<div className="rounded-md border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-950 dark:text-amber-100">
					Uploading replaces all chunks in the{' '}
					<strong>{displayCollection}</strong> collection. Only one document is
					kept at a time.
				</div>

				{isNewCollection && isCollectionValid && (
					<div className="rounded-md border border-blue-500/40 bg-blue-500/10 px-4 py-3 text-sm text-blue-950 dark:text-blue-100">
						New collection &mdash; will be created on upload.
					</div>
				)}

				{selectedCollection && (
					<div className="rounded-md border bg-muted/40 px-4 py-3 text-sm">
						<p>
							<span className="text-muted-foreground">Collection:</span>{' '}
							{selectedCollection.name}
						</p>
						<p>
							<span className="text-muted-foreground">Chunks:</span>{' '}
							{selectedCollection.chunkCount}
						</p>
						{selectedCollection.sourceUri && (
							<p>
								<span className="text-muted-foreground">Current file:</span>{' '}
								{selectedCollection.sourceUri}
							</p>
						)}
						{selectedCollection.embeddingModel && (
							<p>
								<span className="text-muted-foreground">Embeddings:</span>{' '}
								{selectedCollection.embeddingModel}
								{selectedCollection.dimensions
									? ` (${selectedCollection.dimensions}d)`
									: ''}
							</p>
						)}
					</div>
				)}

				<div className="space-y-3">
					<input
						ref={inputRef}
						type="file"
						accept=".md,.txt,.markdown,text/plain,text/markdown"
						className="w-full min-w-0 text-sm file:mr-3 file:rounded-md file:border-0 file:bg-primary file:px-3 file:py-1.5 file:text-sm file:font-medium file:text-primary-foreground hover:file:bg-primary/90"
						onChange={handleFileChange}
						disabled={uploading}
					/>
					<div className="flex justify-end">
						<Button
							onClick={handleUpload}
							disabled={
								uploading || !selectedName || !isCollectionValid
							}
						>
							{uploading ? (
								<>
									<Upload className="h-4 w-4 animate-pulse" />
									Uploading…
								</>
							) : (
								<>
									<FileUp className="h-4 w-4" />
									Upload &amp; index
								</>
							)}
						</Button>
					</div>
				</div>

				{selectedName && !uploading && (
					<p className="break-all text-xs text-muted-foreground">
						Ready: {selectedName}
					</p>
				)}
			</CardContent>
		</Card>
	)
}
