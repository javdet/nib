export const MIN_VISIBLE_MS = 2000

export interface ToolActivityState {
	visible: boolean
	shownAt: number | null
}

export type ToolActivityAction =
	| { type: 'tools_start'; now: number }
	| { type: 'tools_end'; now: number }
	| { type: 'hide' }
	| { type: 'reset' }

export const initialToolActivityState: ToolActivityState = {
	visible: false,
	shownAt: null,
}

/** Delay before hiding, so a sub-second tool call stays readable. */
export function hideDelay(shownAt: number, now: number): number {
	return Math.max(0, MIN_VISIBLE_MS - (now - shownAt))
}

export function reduceToolActivity(
	state: ToolActivityState,
	action: ToolActivityAction,
): ToolActivityState {
	switch (action.type) {
		case 'tools_start':
			if (state.visible) {
				return state
			}
			return { visible: true, shownAt: action.now }
		case 'tools_end':
			if (!state.visible || state.shownAt == null) {
				return state
			}
			return state
		case 'hide':
			return initialToolActivityState
		case 'reset':
			return initialToolActivityState
		default:
			return state
	}
}
