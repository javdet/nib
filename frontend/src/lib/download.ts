export function downloadTextFile(
	name: string,
	content: string,
	mime = 'text/markdown',
): void {
	const blob = new Blob([content], { type: `${mime};charset=utf-8` })
	const url = URL.createObjectURL(blob)
	const anchor = document.createElement('a')
	anchor.href = url
	anchor.download = name
	anchor.style.display = 'none'
	document.body.appendChild(anchor)
	anchor.click()
	document.body.removeChild(anchor)
	URL.revokeObjectURL(url)
}
