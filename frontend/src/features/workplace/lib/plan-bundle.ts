import { planFileBaseName } from './plan-markdown'

// These mirror service.PlanBundleFormat and PlanBundleVersion. The backend is
// what validates a file; checking here only lets an obviously wrong pick fail
// before a large upload.
export const PLAN_BUNDLE_FORMAT = 'nib-plan'
export const PLAN_BUNDLE_VERSION = 1
export const PLAN_BUNDLE_EXTENSION = '.nib'

export function planBundleFileName(title: string): string {
	return `${planFileBaseName(title)}${PLAN_BUNDLE_EXTENSION}`
}

/**
 * Parses the text of a .nib file into the document the import endpoint takes,
 * or throws with a message fit to show the operator.
 */
export function parsePlanBundle(text: string): Record<string, unknown> {
	let doc: unknown
	try {
		doc = JSON.parse(text)
	} catch {
		throw new Error('The file is not a nib plan: it is not valid JSON.')
	}

	if (!doc || typeof doc !== 'object' || Array.isArray(doc)) {
		throw new Error('The file is not a nib plan.')
	}
	const bundle = doc as Record<string, unknown>
	if (bundle.format !== PLAN_BUNDLE_FORMAT) {
		throw new Error('The file is not a nib plan.')
	}
	if (
		typeof bundle.version !== 'number' ||
		bundle.version > PLAN_BUNDLE_VERSION
	) {
		throw new Error(
			'The plan was exported by a newer version of nib. Upgrade nib to import it.',
		)
	}
	return bundle
}
