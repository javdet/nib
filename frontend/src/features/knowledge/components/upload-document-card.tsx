import { useRef, useState } from 'react'
import {
	Eye,
	FileUp,
	Loader2,
	Sparkles,
	TriangleAlert,
	Upload,
} from 'lucide-react'
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

// Chunks, current file and embedding model read as one muted line beside the
// picker, so the card does not grow a metadata block per collection.
function collectionSummary(collection: KnowledgeCollection): string {
	const parts = [`${collection.chunkCount} chunks`]
	if (collection.sourceUri) parts.push(collection.sourceUri)
	if (collection.embeddingModel) {
		const dimensions = collection.dimensions
			? ` (${collection.dimensions}d)`
			: ''
		parts.push(`${collection.embeddingModel}${dimensions}`)
	}
	return parts.join(' · ')
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
	const summary = selectedCollection
		? collectionSummary(selectedCollection)
		: null

	return (
		<Card>
			<CardHeader className="flex flex-row items-start justify-between gap-4 space-y-0">
				<div className="min-w-0">
					<CardTitle>Document upload</CardTitle>
					<p className="mt-1.5 flex items-start gap-1.5 text-sm text-muted-foreground">
						<TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0 text-amber-600 dark:text-amber-500" />
						<span>
							Uploading or generating replaces all chunks in{' '}
							<strong className="font-medium text-foreground">
								{displayCollection}
							</strong>
							{' — '}only one document is kept at a time.
						</span>
					</p>
				</div>
				<Button
					variant="outline"
					size="sm"
					className="shrink-0"
					onClick={() => setViewOpen(true)}
					disabled={!isValidKnowledgeCollectionName(displayCollection)}
				>
					<Eye className="h-4 w-4" />
					View current
				</Button>
			</CardHeader>
			<CardContent className="space-y-4">
				<div className="space-y-2">
					<Label htmlFor="kb-collection">Collection</Label>
					<div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
						<div className="w-64 max-w-full shrink-0">
							<CollectionSelect
								collections={collections}
								value={collection}
								onChange={onCollectionChange}
								disabled={busy}
							/>
						</div>
						{summary !== null ? (
							<p
								className="min-w-[10rem] flex-1 truncate text-xs text-muted-foreground"
								title={summary}
							>
								{summary}
							</p>
						) : (
							isNewCollection &&
							isCollectionValid && (
								<p className="min-w-[10rem] flex-1 truncate text-xs text-muted-foreground">
									New collection &mdash; created on upload.
								</p>
							)
						)}
					</div>
				</div>

				<div className="flex flex-wrap items-center gap-2">
					<Input
						ref={inputRef}
						type="file"
						aria-label="Document file"
						accept=".md,.txt,.markdown,text/plain,text/markdown"
						className="min-w-[14rem] flex-1 cursor-pointer py-1.5"
						onChange={handleFileChange}
						disabled={busy}
					/>
					<Button
						className="ml-auto shrink-0"
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

				<div className="space-y-2 border-t pt-4">
					<Label htmlFor="kb-repositories">Repositories</Label>
					<div className="flex flex-wrap items-center gap-2">
						<Input
							id="kb-repositories"
							value={repositories}
							onChange={(e) => onRepositoriesChange(e.target.value)}
							placeholder="myorg/infra, https://github.com/myorg/helm-charts"
							className="min-w-[14rem] flex-1"
							disabled={busy}
							autoComplete="off"
						/>
						<Button
							className="ml-auto shrink-0"
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
					<p className="text-xs text-muted-foreground">
						Comma-separated, as <code>owner/repo</code> or a full URL. The agent
						reads these and writes the document to{' '}
						<strong className="font-medium">{displayCollection}</strong>.
					</p>
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
