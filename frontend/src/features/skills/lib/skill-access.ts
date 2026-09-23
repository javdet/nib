import type { SkillMeta } from '../api/skills'

/** List badges for the skills page: only the restricted settings earn one. */
export function skillAccessBadges(skills: SkillMeta[]): Record<string, string> {
	const badges: Record<string, string> = {}
	for (const skill of skills) {
		if (skill.access === 'explicit' || skill.access === 'disabled') {
			badges[skill.name] = skill.access
		}
	}
	return badges
}
