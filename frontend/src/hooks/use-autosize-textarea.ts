import { useCallback, useEffect, useLayoutEffect, useState } from 'react'
import type { RefObject } from 'react'

/**
 * Grows a textarea upward with its content, up to `maxRatio` of the container's
 * height, and shrinks back as content is removed (including the reset to empty
 * after a send).
 *
 * The cap is snapped down to a whole number of text lines: an arbitrary pixel
 * cap would leave the frame cutting a line of letters in half.
 */
export function useAutosizeTextarea(
	textareaRef: RefObject<HTMLTextAreaElement | null>,
	containerRef: RefObject<HTMLElement | null>,
	value: string,
	maxRatio = 0.5,
) {
	const [containerHeight, setContainerHeight] = useState(0)

	useEffect(() => {
		const container = containerRef.current
		if (!container) return

		const observer = new ResizeObserver((entries) => {
			const entry = entries[0]
			if (entry) setContainerHeight(entry.contentRect.height)
		})
		observer.observe(container)
		return () => observer.disconnect()
	}, [containerRef])

	const resize = useCallback(() => {
		const el = textareaRef.current
		if (!el) return

		const styles = getComputedStyle(el)
		const lineHeight = parseFloat(styles.lineHeight) ||
			parseFloat(styles.fontSize) * 1.5
		// scrollHeight covers padding but not borders, while box-sizing:border-box
		// makes the height we set include them.
		const chrome =
			parseFloat(styles.paddingTop) +
			parseFloat(styles.paddingBottom) +
			parseFloat(styles.borderTopWidth) +
			parseFloat(styles.borderBottomWidth)

		// A measurement of the content alone needs the previous height gone.
		el.style.height = 'auto'
		const contentHeight =
			el.scrollHeight + parseFloat(styles.borderTopWidth) +
			parseFloat(styles.borderBottomWidth)

		const cap = containerHeight * maxRatio
		const visibleLines = Math.max(
			1,
			Math.floor((cap - chrome) / lineHeight),
		)
		const maxHeight = visibleLines * lineHeight + chrome

		const height = cap > 0 ? Math.min(contentHeight, maxHeight) : contentHeight
		el.style.height = `${height}px`
		el.style.overflowY = contentHeight > height ? 'auto' : 'hidden'
	}, [textareaRef, containerHeight, maxRatio])

	useLayoutEffect(resize, [resize, value])
}
