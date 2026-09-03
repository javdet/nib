import type { Dialog } from '@/features/dialogs/api/dialogs'

const UNTITLED_BY_MODE: Record<string, string> = {
	main: 'New plan',
	// decompose keeps the label for a plan created before the orchestrator: its
	// own transcripts are titled when they are created.
	decompose: 'New plan',
	plan: 'New plan',
	execute: 'New execution',
	incident: 'New incident',
}

export function dialogDisplayTitle(
	dialog: Pick<Dialog, 'title' | 'mode'>,
): string {
	return dialog.title.trim() || UNTITLED_BY_MODE[dialog.mode] || 'New chat'
}
