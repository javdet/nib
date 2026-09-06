import { useCallback, useEffect, useLayoutEffect, useState } from 'react'
import type { RefObject } from 'react'

interface AutosizeTextareaOptions {
	textareaRef: RefObject<HTMLTextAreaElement | null>
	/** Element whose height the cap is a fraction of. */
	containerRef: RefObject<HTMLElement | null>
	/** Optional frame around the textarea, whose padding counts against the cap. */
	frameRef?: RefObject<HTMLElement | null>
	value: string
	maxRatio?: number
}

/**
 * Grows a textarea upward with its content, up to `maxRatio` of the container's
 * height, and shrinks back as content is removed (including the reset to empty
 * after a send).
 *
 * The cap is snapped down to a whole number of text lines: an arbitrary pixel
 * cap would leave the frame cutting a line of letters in half. That only holds
 * while the textarea itself carries no vertical padding — padding scrolls with
 * the content, so it would expose a sliver of the next line past the last whole
 * one. Hence `frameRef`: the padding lives on the frame instead.
 */
export function useAutosizeTextarea({
	textareaRef,
	containerRef,
	frameRef,
	value,
	maxRatio = 0.5,
}: AutosizeTextareaOptions) {
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
		const lineHeight =
			parseFloat(styles.lineHeight) || parseFloat(styles.fontSize) * 1.5
		const borders =
			parseFloat(styles.borderTopWidth) + parseFloat(styles.borderBottomWidth)
		// scrollHeight covers padding but not borders, while box-sizing:border-box
		// makes the height we set include them.
		const chrome =
			parseFloat(styles.paddingTop) + parseFloat(styles.paddingBottom) + borders
		const frame = frameRef?.current
		const frameChrome = frame ? frame.offsetHeight - el.offsetHeight : 0

		// A measurement of the content alone needs the previous height gone, which
		// scrolls an overflowing draft back to the top — keep where the reader was.
		const { scrollTop } = el
		el.style.height = 'auto'
		const contentHeight = el.scrollHeight + borders

		const cap = containerHeight * maxRatio - frameChrome
		const wholeLines = Math.max(1, Math.floor((cap - chrome) / lineHeight))
		const maxHeight = wholeLines * lineHeight + chrome

		const height = cap > 0 ? Math.min(contentHeight, maxHeight) : contentHeight
		el.style.height = `${height}px`
		el.style.overflowY = contentHeight > height ? 'auto' : 'hidden'
		el.scrollTop = scrollTop
	}, [textareaRef, frameRef, containerHeight, maxRatio])

	useLayoutEffect(resize, [resize, value])
}
