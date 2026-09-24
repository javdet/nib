export interface SearchableEntry {
	name: string
	description?: string
	scope: string
	scopeName?: string
}

// The whole variable and secret list is fetched at once, so search filters in
// memory rather than going back to the API the way the plan list does.
export function filterByQuery<T extends SearchableEntry>(
	entries: T[],
	query: string,
): T[] {
	const needle = query.trim().toLowerCase()
	if (needle === '') {
		return entries
	}
	return entries.filter((entry) =>
		[entry.name, entry.description, entry.scope, entry.scopeName].some(
			(field) => field?.toLowerCase().includes(needle),
		),
	)
}
