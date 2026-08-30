import { api } from '@/lib/api-client'

export {
	DEFAULT_COLLECTION,
	KNOWLEDGE_COLLECTION_NAME_PATTERN,
	collectionForProject,
	isValidKnowledgeCollectionName,
} from '../lib/collection-name'

export interface KnowledgeConnection {
	connectionUri: string
}

export interface KnowledgeStatus {
	collectionName: string
	connectionUri: string
	chunkCount: number
	sourceUri?: string
	dimensions?: number
	embeddingModel?: string
}

export interface KnowledgeCollection {
	name: string
	chunkCount: number
	sourceUri?: string
	dimensions?: number
	embeddingModel?: string
}

export interface KnowledgeUploadResult {
	filename: string
	chunkCount: number
	collection: string
}

/** Where the body of a viewed knowledge base came from. */
export type KnowledgeDocumentSource = 'uploaded' | 'template'

export interface KnowledgeDocument {
	collection: string
	/** Original upload filename; absent for the built-in template. */
	filename?: string
	content: string
	source: KnowledgeDocumentSource
	/** Absent for the built-in template. */
	updatedAt?: string
}

export interface UpdateConnectionPayload {
	connectionUri: string
}

export function getConnection() {
	return api.get<KnowledgeConnection>('/knowledge/connection')
}

export function updateConnection(data: UpdateConnectionPayload) {
	return api.put<KnowledgeConnection>('/knowledge/connection', data)
}

export function listCollections() {
	return api.get<KnowledgeCollection[]>('/knowledge/collections')
}

export function getStatus(collection?: string) {
	const params = collection?.trim()
		? `?collection=${encodeURIComponent(collection.trim())}`
		: ''
	return api.get<KnowledgeStatus>(`/knowledge/status${params}`)
}

export async function uploadDocument(
	file: File,
	collection: string,
): Promise<KnowledgeUploadResult> {
	const form = new FormData()
	form.append('file', file)
	form.append('collection', collection)
	return api.upload<KnowledgeUploadResult>('/knowledge/documents', form)
}

/**
 * Reads the source document behind a collection. The backend answers with the
 * built-in skeleton when nothing has been uploaded, so this never 404s.
 */
export function getDocument(collection: string) {
	const params = collection.trim()
		? `?collection=${encodeURIComponent(collection.trim())}`
		: ''
	return api.get<KnowledgeDocument>(`/knowledge/documents${params}`)
}

export interface DiscussPrompt {
	name: string
	content: string
}

export function getDiscussPrompt() {
	return api.get<DiscussPrompt>('/system-prompts/discuss')
}

export function updateDiscussPrompt(content: string) {
	return api.put<void>('/system-prompts/discuss', { content })
}
