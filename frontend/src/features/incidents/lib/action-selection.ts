import type { Postmortem, PostmortemAction } from '../api/incidents'

/** An action can be handed to a plan while it is still to do. */
export function isActionSelectable(action: PostmortemAction): boolean {
	return action.status === 'todo' && !action.planId
}

/**
 * Drops from a selection every id that is no longer selectable: an agent may
 * have rewritten the list, or another tab handed the action to a plan, since
 * the operator ticked it.
 */
export function pruneSelection(selected: ReadonlySet<string>, doc: Postmortem): Set<string> {
	const selectable = new Set(
		[...doc.shortTerm, ...doc.longTerm].filter(isActionSelectable).map((a) => a.id),
	)
	return new Set([...selected].filter((id) => selectable.has(id)))
}
