export const THINKING_PHRASES = [
	'Thinking',
	'Planning',
	'Reasoning',
	'Pondering',
	'Meditating',
	'Musing',
	'Cogitating',
	'Deliberating',
	'Contemplating',
	'Ruminating',
	'Analyzing',
	'Strategizing',
] as const

export function thinkingPhrase(turn: number): string {
	const len = THINKING_PHRASES.length
	// The double modulo keeps the index in range for negative turns too, which
	// noUncheckedIndexedAccess cannot prove.
	return THINKING_PHRASES[((turn % len) + len) % len]!
}
