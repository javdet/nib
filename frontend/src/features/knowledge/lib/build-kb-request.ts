/** The skill that turns a project's repositories into its knowledge base document. */
export const BUILD_KNOWLEDGE_BASE_SKILL = 'build-knowledge-base'

/**
 * Splits the comma-separated repositories field into individual entries.
 *
 * Deliberately shape-agnostic: the skill accepts both `owner/repo` and full
 * URLs, and nothing downstream parses an entry, so rejecting anything here
 * would only turn inputs the agent handles into errors the operator cannot fix.
 */
export function parseRepositoryList(raw: string): string[] {
	const seen = new Set<string>()
	const out: string[] = []
	for (const part of raw.split(',')) {
		const repo = part.trim()
		if (!repo || seen.has(repo)) continue
		seen.add(repo)
		out.push(repo)
	}
	return out
}

/**
 * The message that invokes the build-knowledge-base skill unambiguously.
 *
 * Skills are not slash commands - the text goes to the model verbatim, and only
 * `included` skills are named in the discuss system prompt. Asking for the
 * get_skill call by name takes the guess out of the invocation, and passing both
 * parameters the skill requires stops it from asking for them and halting.
 */
export function buildKnowledgeBaseRequest(
	project: string,
	repositories: string[],
): string {
	return [
		`Call get_skill with name "${BUILD_KNOWLEDGE_BASE_SKILL}" and follow its instructions exactly.`,
		'',
		`project: ${project}`,
		`repositories: ${repositories.join(', ')}`,
	].join('\n')
}
