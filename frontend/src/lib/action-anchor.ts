/**
 * Links to a single row of an action plan.
 *
 * A row is addressed by its storage key (`s3.step1`, `s0.check0`,
 * `rollback.2`), the same positional key the backend uses, so a link is as
 * stable as the "4.2" label beside it: reordering moves both.
 *
 * Two forms exist. The plan page reads `#action-{key}` out of its own URL,
 * which is what an operator copies and shares. A chat message instead carries
 * `#action:{planId}:{key}`, because the chat panel shows the plan chat on any
 * page and has to know which plan to open.
 */

export const ROLLBACK_KEY_RE = /^rollback\.(\d+)$/
export const STAGE_ITEM_KEY_RE = /^s(\d+)\.(step|check)(\d+)$/

const ANCHOR_PREFIX = 'action-'
const HREF_PREFIX = '#action:'

export function isActionPlanKey(key: string): boolean {
	return ROLLBACK_KEY_RE.test(key) || STAGE_ITEM_KEY_RE.test(key)
}

/** The DOM id of a row, and the fragment of the plan page that focuses it. */
export function actionAnchorId(key: string): string {
	return `${ANCHOR_PREFIX}${key}`
}

/** Reads the row key out of the plan page's hash, or null when it names none. */
export function actionKeyFromHash(hash: string | undefined): string | null {
	const fragment = hash?.startsWith('#') ? hash.slice(1) : hash
	if (!fragment?.startsWith(ANCHOR_PREFIX)) return null
	const key = decodeURIComponent(fragment.slice(ANCHOR_PREFIX.length))
	return isActionPlanKey(key) ? key : null
}

/** The page that shows a row, with the row focused. */
export function actionPagePath(planId: string, key: string): string {
	return `/workplace/${planId}#${actionAnchorId(key)}`
}

/** Builds the href a chat message uses to point at a row. */
export function actionHref(planId: string, key: string): string {
	return `${HREF_PREFIX}${planId}:${key}`
}

export interface ActionLink {
	planId: string
	key: string
}

/** Reads a chat message's link to a row, or null when it is any other link. */
export function actionLinkFromHref(href: string | undefined): ActionLink | null {
	if (!href?.startsWith(HREF_PREFIX)) return null
	const rest = href.slice(HREF_PREFIX.length)
	const sep = rest.lastIndexOf(':')
	if (sep <= 0) return null
	const planId = rest.slice(0, sep).trim()
	const key = rest.slice(sep + 1).trim()
	if (!planId || !isActionPlanKey(key)) return null
	return { planId, key }
}
