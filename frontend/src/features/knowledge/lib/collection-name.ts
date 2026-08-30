/** Collections are stored as {name}.md, so the backend enforces this shape too. */
export const KNOWLEDGE_COLLECTION_NAME_PATTERN =
	/^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$/

/** The collection used when no project is selected. */
export const DEFAULT_COLLECTION = 'default'

/** The sentinel name the header selectors fall back to (see use-selectable-resource). */
const ANY_PROJECT_NAME = 'any'

export function isValidKnowledgeCollectionName(name: string): boolean {
	const trimmed = name.trim()
	return trimmed.length > 0 && KNOWLEDGE_COLLECTION_NAME_PATTERN.test(trimmed)
}

/**
 * The knowledge base collection a project maps to.
 *
 * Projects come from config.yaml and may be named something the backend will
 * not accept as a collection; that and the "any" sentinel both fall back to the
 * shared default rather than sending an upload the server would reject.
 */
export function collectionForProject(
	projectName: string | null | undefined,
): string {
	const trimmed = projectName?.trim() ?? ''
	if (!trimmed || trimmed === ANY_PROJECT_NAME) return DEFAULT_COLLECTION
	return isValidKnowledgeCollectionName(trimmed)
		? trimmed
		: DEFAULT_COLLECTION
}
