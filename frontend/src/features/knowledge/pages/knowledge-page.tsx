import { useState, useEffect, useCallback, useRef } from 'react'
import { extractErrorMessage } from '@/lib/api-client'
import {
	type KnowledgeCollection,
	collectionForProject,
	listCollections,
	uploadDocument,
} from '../api/knowledge'
import { useProject } from '@/features/projects/project-context'
import { CompanyInfoCard } from '@/features/company/components/company-info-card'
import { UploadDocumentCard } from '../components/upload-document-card'
import { DiscussPromptCard } from '../components/discuss-prompt-card'

export function KnowledgePage() {
	const { selectedProject } = useProject()
	const projectCollection = collectionForProject(selectedProject?.name)

	const [collections, setCollections] = useState<KnowledgeCollection[]>([])
	const [collection, setCollection] = useState(projectCollection)
	const [loading, setLoading] = useState(true)
	const [uploading, setUploading] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const errorRef = useRef<HTMLDivElement>(null)

	const refreshCollections = useCallback(async () => {
		try {
			const items = await listCollections()
			setCollections(items)
		} catch {
			setCollections([])
		}
	}, [])

	useEffect(() => {
		async function load() {
			setLoading(true)
			setError(null)
			await refreshCollections()
			setLoading(false)
		}
		load()
	}, [refreshCollections])

	useEffect(() => {
		if (!error) return
		errorRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
	}, [error])

	// Each project keeps its own knowledge base, so switching projects in the
	// header switches the collection under it. An ad-hoc collection picked here
	// stands until the project changes again.
	useEffect(() => {
		setCollection(projectCollection)
	}, [projectCollection])

	const handleUpload = useCallback(
		async (file: File) => {
			setUploading(true)
			setError(null)
			try {
				await uploadDocument(file, collection)
				await refreshCollections()
			} catch (err) {
				setError(extractErrorMessage(err))
			} finally {
				setUploading(false)
			}
		},
		[collection, refreshCollections],
	)

	if (loading) {
		return (
			<div className="flex items-center justify-center py-12">
				<p className="text-sm text-muted-foreground">Loading...</p>
			</div>
		)
	}

	return (
		<div className="space-y-6">
			<h2 className="text-2xl font-bold tracking-tight">Knowledge Base</h2>

			{error && (
				<div
					ref={errorRef}
					className="scroll-mt-24 rounded-md border border-destructive/50 bg-destructive/10 px-4 py-3 text-sm text-destructive"
				>
					{error}
				</div>
			)}

			<CompanyInfoCard />

			<UploadDocumentCard
				collections={collections}
				collection={collection}
				onCollectionChange={setCollection}
				uploading={uploading}
				onUpload={handleUpload}
			/>

			<DiscussPromptCard />
		</div>
	)
}
