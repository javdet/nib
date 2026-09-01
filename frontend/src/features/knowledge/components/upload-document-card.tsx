import { useRef, useState } from 'react'
import { Eye, FileUp, Loader2, Sparkles, Upload } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
	type KnowledgeCollection,
	isValidKnowledgeCollectionName,
} from '../api/knowledge'
import { parseRepositoryList } from '../lib/build-kb-request'
import { CollectionSelect } from './collection-select'
import { ViewDocumentDialog } from './view-document-dialog'

interface UploadDocumentCardProps {
	collections: KnowledgeCollection[]
	collection: string
	onCollectionChange: (name: string) => void
	uploading: boolean
	onUpload: (file: File) => void
	repositories: string
	onRepositoriesChange: (value: string) => void
	generating: boolean
	onGenerate: () => void
}

export function UploadDocumentCard({
	collections,
	collection,
	onCollectionChange,
	uploading,
	onUpload,
	repositories,
	onRepositoriesChange,
	generating,
	onGenerate,
}: UploadDocumentCardProps) {
	const inputRef = useRef<HTMLInputElement>(null)
	const [selectedName, setSelectedName] = useState<string | null>(null)
	const [viewOpen, setViewOpen] = useState(false)

	const trimmedCollection = collection.trim()
	const selectedCollection = collections.find(
		(item) => item.name === trimmedCollection,
	)
	const isNewCollection =
		trimmedCollection.length > 0 && selectedCollection === undefined
	const isCollectionValid = isValidKnowledgeCollectionName(collection)

	// Both paths write the whole collection, so neither may run while the other does.
	const busy = uploading || generating
	const hasRepositories = parseRepositoryList(repositories).length > 0

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
						disabled={busy}
					/>
				</div>

				<div className="rounded-md border border-amber-500/40 bg-amber-500/10 px-4 py-3 text-sm text-amber-950 dark:text-amber-100">
					Uploading or generating replaces all chunks in the{' '}
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
					<Input
						ref={inputRef}
						type="file"
						accept=".md,.txt,.markdown,text/plain,text/markdown"
						className="min-w-0 cursor-pointer py-1.5"
						onChange={handleFileChange}
						disabled={busy}
					/>
					<div className="flex justify-end gap-2">
						<Button
							variant="outline"
							onClick={() => setViewOpen(true)}
							disabled={!isValidKnowledgeCollectionName(displayCollection)}
						>
							<Eye className="h-4 w-4" />
							View current
						</Button>
						<Button
							onClick={handleUpload}
							disabled={busy || !selectedName || !isCollectionValid}
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

				<div className="space-y-3 border-t pt-4">
					<div className="space-y-2">
						<Label htmlFor="kb-repositories">Repositories</Label>
						<Input
							id="kb-repositories"
							value={repositories}
							onChange={(e) => onRepositoriesChange(e.target.value)}
							placeholder="myorg/infra, https://github.com/myorg/helm-charts"
							disabled={busy}
							autoComplete="off"
						/>
						<p className="text-xs text-muted-foreground">
							Comma-separated, as <code>owner/repo</code> or a full URL. The
							agent reads these and writes the document to{' '}
							<strong>{displayCollection}</strong>.
						</p>
					</div>
					<div className="flex justify-end">
						<Button
							onClick={onGenerate}
							disabled={busy || !hasRepositories || !isCollectionValid}
						>
							{generating ? (
								<>
									<Loader2 className="h-4 w-4 animate-spin" />
									Starting…
								</>
							) : (
								<>
									<Sparkles className="h-4 w-4" />
									Generate knowledge base
								</>
							)}
						</Button>
					</div>
				</div>

				<ViewDocumentDialog
					collection={displayCollection}
					open={viewOpen}
					onOpenChange={setViewOpen}
				/>
			</CardContent>
		</Card>
	)
}
