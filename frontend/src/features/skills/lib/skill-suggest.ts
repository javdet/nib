import type { SkillMeta } from '../api/skills'

// Skill names are file stems on disk, so a name can only hold these characters.
const SKILL_QUERY_RE = /^\/([A-Za-z0-9._-]*)$/

// Guards the DOM against a large skills directory; the menu window itself only
// shows a handful of rows at a time.
export const MAX_SKILL_SUGGESTIONS = 50

/**
 * The skill name being typed, or null when the draft is not a bare slash command.
 *
 * The whole draft has to be the token: a leading `/` is only a command at the very
 * start of an empty message, and the first character a skill name cannot hold means
 * the operator moved on to prose. That is what closes the menu, with no extra state.
 */
export function parseSkillQuery(draft: string): string | null {
	const match = SKILL_QUERY_RE.exec(draft)
	if (!match) return null
	return match[1] ?? ''
}

/** Active slash query, or null when the menu is dismissed or the draft is not a token. */
export function skillMenuQuery(
	draft: string,
	dismissed: boolean,
): string | null {
	if (dismissed) return null
	return parseSkillQuery(draft)
}

/**
 * Whether a draft edit should clear the dismiss flag.
 *
 * Re-arms when the slash token disappears (so Escape-then-delete works) and when
 * the draft newly becomes a token (so blur-then-type-slash works).
 */
export function shouldRearmSkillMenu(prev: string, next: string): boolean {
	if (!next.startsWith('/')) return true
	if (parseSkillQuery(prev) === null && parseSkillQuery(next) !== null) {
		return true
	}
	return false
}

/** `build-knowledge-base` -> ['build', 'knowledge', 'base'] for word-prefix hits. */
function segmentsOf(name: string): string[] {
	return name.split(/[._-]+/).filter(Boolean)
}

/**
 * Skills whose name or description matches `query`, best match first.
 *
 * Tiers rather than a fuzzy score: the operator is typing a name they half remember,
 * so a name prefix has to win outright and ties have to stay alphabetical. A
 * subsequence score reorders the rows unpredictably between keystrokes.
 */
export function matchSkills(
	skills: SkillMeta[],
	query: string,
	limit = MAX_SKILL_SUGGESTIONS,
): SkillMeta[] {
	const needle = query.trim().toLowerCase()
	const byName = [...skills].sort((a, b) => a.name.localeCompare(b.name))
	if (!needle) {
		return byName.slice(0, limit)
	}

	const ranked: { skill: SkillMeta; tier: number }[] = []
	for (const skill of byName) {
		const name = skill.name.toLowerCase()
		const tier = name.startsWith(needle)
			? 0
			: segmentsOf(name).some((segment) => segment.startsWith(needle))
				? 1
				: name.includes(needle)
					? 2
					: skill.description.toLowerCase().includes(needle)
						? 3
						: -1
		if (tier >= 0) {
			ranked.push({ skill, tier })
		}
	}
	// Sort is stable in every runtime we target, so each tier keeps the alphabetical
	// order established above.
	ranked.sort((a, b) => a.tier - b.tier)
	return ranked.slice(0, limit).map((entry) => entry.skill)
}

/** The draft after accepting a suggestion; the trailing space starts the argument. */
export function applySkillSelection(name: string): string {
	return `/${name} `
}
