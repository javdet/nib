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

// An execute dialog carries out one action of a plan that is already written,
// so "Planning" or "Strategizing" there reads as if the plan were being redone.
export const EXECUTING_PHRASES = [
	'Executing',
	'Working',
	'Running',
	'Applying',
	'Carrying out',
	'Processing',
	'Performing',
	'Implementing',
	'Operating',
	'Checking',
] as const

export function phrasesForMode(mode?: string | null): readonly string[] {
	return mode === 'execute' ? EXECUTING_PHRASES : THINKING_PHRASES
}

export function thinkingPhrase(turn: number, mode?: string | null): string {
	const phrases = phrasesForMode(mode)
	const len = phrases.length
	// The double modulo keeps the index in range for negative turns too, which
	// noUncheckedIndexedAccess cannot prove.
	return phrases[((turn % len) + len) % len]!
}
