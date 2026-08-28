import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs))
}

export function formatDialogDate(iso: string): string {
	const date = new Date(iso)
	if (Number.isNaN(date.getTime())) return ''
	const now = new Date()
	const sameDay =
		date.getFullYear() === now.getFullYear() &&
		date.getMonth() === now.getMonth() &&
		date.getDate() === now.getDate()
	if (sameDay) return 'today'
	const month = date.toLocaleString('en-US', { month: 'short' })
	const day = date.getDate()
	if (date.getFullYear() === now.getFullYear()) {
		return `${month} ${day}`
	}
	return `${date.getFullYear()} ${month} ${day}`
}

export function findLastIndex<T>(
	items: T[],
	predicate: (item: T, index: number) => boolean,
): number {
	for (let i = items.length - 1; i >= 0; i--) {
		const item = items[i]
		if (item !== undefined && predicate(item, i)) {
			return i
		}
	}
	return -1
}
