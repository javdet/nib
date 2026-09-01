import { useState, useEffect, useCallback, useRef } from 'react'
import { extractErrorMessage } from '@/lib/api-client'
import {
	type KnowledgeCollection,
	DEFAULT_COLLECTION,
	collectionForProject,
	listCollections,
	uploadDocument,
} from '../api/knowledge'
import { useProject } from '@/features/projects/project-context'
import { useMode } from '@/features/modes/mode-context'
import { useDialog } from '@/features/dialogs/dialog-context'
import { createDialog } from '@/features/dialogs/api/dialogs'
import { CompanyInfoCard } from '@/features/company/components/company-info-card'
import { UploadDocumentCard } from '../components/upload-document-card'
import { DiscussPromptCard } from '../components/discuss-prompt-card'
import {
	buildKnowledgeBaseRequest,
	parseRepositoryList,
} from '../lib/build-kb-request'

export function KnowledgePage() {
	const { selectedProject } = useProject()
	const { selectMode } = useMode()
	const { setActiveDialogId, bumpDialogsVersion, enqueuePendingMessage } =
		useDialog()
	const projectCollection = collectionForProject(selectedProject?.name)

	const [collections, setCollections] = useState<KnowledgeCollection[]>([])
	const [collection, setCollection] = useState(projectCollection)
	const [repositories, setRepositories] = useState('')
	const [loading, setLoading] = useState(true)
	const [uploading, setUploading] = useState(false)
	const [generating, setGenerating] = useState(false)
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

	// Repositories belong to the collection they were typed for, and the list is
	// deliberately not persisted, so retarget means retype.
	useEffect(() => {
		setRepositories('')
	}, [collection])

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

	const handleGenerate = useCallback(async () => {
		const repos = parseRepositoryList(repositories)
		if (repos.length === 0) return

		const target = collection.trim() || DEFAULT_COLLECTION
		setGenerating(true)
		setError(null)
		try {
			// Only discuss carries the skills list and the get_kb_document /
			// update_kb tools, and the system prompt is frozen on a dialog's first
			// message - so the mode has to be settled before anything is sent.
			selectMode('discuss')
			const dialog = await createDialog({
				mode: 'discuss',
				title: `Knowledge base: ${target}`,
			})
			// The chat panel picks this up once it has loaded the (empty)
			// transcript for the newly active dialog and sends it for us.
			enqueuePendingMessage({
				dialogId: dialog.id,
				text: buildKnowledgeBaseRequest(target, repos),
			})
			setActiveDialogId(dialog.id)
			bumpDialogsVersion()
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setGenerating(false)
		}
	}, [
		repositories,
		collection,
		selectMode,
		enqueuePendingMessage,
		setActiveDialogId,
		bumpDialogsVersion,
	])

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
					className="scroll-mt-24 rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive"
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
				repositories={repositories}
				onRepositoriesChange={setRepositories}
				generating={generating}
				onGenerate={() => void handleGenerate()}
			/>

			<DiscussPromptCard />
		</div>
	)
}
