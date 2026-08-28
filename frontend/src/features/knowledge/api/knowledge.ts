import { api } from '@/lib/api-client'

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

export interface UpdateConnectionPayload {
	connectionUri: string
}

export const KNOWLEDGE_COLLECTION_NAME_PATTERN =
	/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$/

export function isValidKnowledgeCollectionName(name: string): boolean {
	const trimmed = name.trim()
	return trimmed.length > 0 && KNOWLEDGE_COLLECTION_NAME_PATTERN.test(trimmed)
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
