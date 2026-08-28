export const OPTION_LONG_THRESHOLD = 48

const BREAK_CHUNK_SIZE = 28
const ZERO_WIDTH_SPACE = '\u200B'

export function shouldStackOptions(options: string[]): boolean {
	return options.some((option) => option.length > OPTION_LONG_THRESHOLD)
}

function insertBreaksInRun(run: string): string {
	if (run.length <= BREAK_CHUNK_SIZE) {
		return run
	}

	const parts: string[] = []
	for (let i = 0; i < run.length; i += BREAK_CHUNK_SIZE) {
		parts.push(run.slice(i, i + BREAK_CHUNK_SIZE))
	}
	return parts.join(ZERO_WIDTH_SPACE)
}

export function formatOptionLabel(text: string): string {
	if (text.length <= OPTION_LONG_THRESHOLD) {
		return text
	}

	return text
		.split(/(\s+)/)
		.map((segment) => {
			if (/^\s+$/.test(segment)) {
				return segment
			}
			return insertBreaksInRun(segment)
		})
		.join('')
}
