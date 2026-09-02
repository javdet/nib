/**
 * Links between dialogs.
 *
 * A message can point at another conversation — a sub-agent's transcript, say.
 * There is no route to a dialog by id, since the app switches conversations
 * through context rather than the URL, so such a link carries a fragment the
 * renderer intercepts instead of an href a browser could follow.
 */
const DIALOG_HREF_PREFIX = '#dialog:'

/** Builds the href a message uses to point at a dialog. */
export function dialogHref(dialogId: string): string {
	return `${DIALOG_HREF_PREFIX}${dialogId}`
}

/** Reads the dialog id out of a link, or null when it is an ordinary one. */
export function dialogIdFromHref(href: string | undefined): string | null {
	if (!href?.startsWith(DIALOG_HREF_PREFIX)) return null
	const id = href.slice(DIALOG_HREF_PREFIX.length).trim()
	return id.length > 0 ? id : null
}
