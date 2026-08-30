const MAX_TOOL_DESCRIPTION_LENGTH = 280
const MAX_TOOL_DESCRIPTION_SENTENCES = 2

/** Sentence boundary: punctuation, space, then a capital letter. */
const sentenceBoundary = /(?<=[.!?])\s+(?=[A-Z])/

function collapseWhitespace(text: string): string {
	return text.replace(/\s+/g, ' ').trim()
}

function clipAtWordBoundary(text: string, maxLength: number): string {
	if (text.length <= maxLength) {
		return text
	}
	const slice = text.slice(0, maxLength)
	const lastSpace = slice.lastIndexOf(' ')
	if (lastSpace > maxLength * 0.6) {
		return `${slice.slice(0, lastSpace).trimEnd()}…`
	}
	return `${slice.trimEnd()}…`
}

function firstSentences(text: string, count: number): string {
	const parts = text.split(sentenceBoundary)
	if (parts.length <= count) {
		return text
	}
	return parts.slice(0, count).join(' ').trim()
}

/** Short catalog description for tooltips; full text stays in search/filter. */
export function summarizeToolDescription(description: string): string {
	const normalized = collapseWhitespace(description)
	if (!normalized) {
		return ''
	}

	const sentences = firstSentences(normalized, MAX_TOOL_DESCRIPTION_SENTENCES)
	return clipAtWordBoundary(sentences, MAX_TOOL_DESCRIPTION_LENGTH)
}
