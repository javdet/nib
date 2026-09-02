/**
 * The sentences the Execute buttons put into the planning chat.
 *
 * Pressing a button sends a message rather than calling an endpoint: the agent
 * reads it and calls `execute_action`, which is the same path a message typed by
 * hand takes. That is what lets an operator write "run step 1.1" in their own
 * words, or in their own language, and get the same behaviour as the button.
 */

/** Asks the agent to run one plan item. */
export function executeActionMessage(number: string): string {
	return `Execute plan item ${number}`
}

/** Asks the agent to abandon whatever is running on an item and start over. */
export function restartActionMessage(number: string): string {
	return `Restart plan item ${number} with a new sub-agent`
}
