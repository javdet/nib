// Mirrors the backend's normalisation closely enough to tell whether an edit
// adds a host: the backend is the one that validates, and adding a host is what
// requires the secret's value to be entered again.
export function parseAllowedHosts(text: string): string[] {
	const hosts = text
		.split(/[\s,]+/)
		.map((h) =>
			h
				.trim()
				.toLowerCase()
				.replace(/\.$/, '')
				.replace(/^\[(.*)\]$/, '$1'),
		)
		.filter((h) => h !== '')
	return [...new Set(hosts)].sort()
}

export function addsAllowedHost(before: string[], after: string[]): boolean {
	const known = new Set(before)
	return after.some((h) => !known.has(h))
}
