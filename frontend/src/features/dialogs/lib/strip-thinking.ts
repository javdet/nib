// Closed `<thinking>` blocks, tolerant of attributes and of casing.
const THINKING_BLOCK = /<thinking\b[^>]*>[\s\S]*?<\/thinking\s*>/gi
// A block the model opened and never closed -- a reply cut off mid-narration
// would otherwise keep its whole tail.
const THINKING_OPEN = /<thinking\b[^>]*>[\s\S]*$/i

/**
 * Removes `<thinking>` narration from an assistant message.
 *
 * The plan and decompose prompts ask the model to open a turn with one, and the
 * backend now cuts it before storing the row -- but transcripts written before
 * that still carry the tags, so the renderer strips them again rather than
 * leaving old plans and executor branches showing scratch reasoning.
 */
export function stripThinking(content: string): string {
	if (!content.toLowerCase().includes('<thinking')) return content
	return content.replace(THINKING_BLOCK, '').replace(THINKING_OPEN, '').trim()
}
