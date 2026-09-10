import type { Page } from '@playwright/test'

/**
 * WCAG contrast measured in the browser, against the *effective* background:
 * nib's surfaces are stacked translucent layers (`bg-primary/10` over a card over
 * the shell), so a naive read of `background-color` measures a transparent pixel.
 * Colours are parsed by painting them into a 1x1 canvas, which handles `oklch()`
 * and `color-mix()` the same as `rgb()`.
 */
const SCRIPT = `
(() => {
	const cv = document.createElement('canvas')
	cv.width = 1
	cv.height = 1
	const ctx = cv.getContext('2d', { willReadFrequently: true })

	function parse(c) {
		if (!c || c === 'transparent' || c === 'none') return null
		ctx.clearRect(0, 0, 1, 1)
		ctx.fillStyle = 'rgba(0,0,0,0)'
		ctx.fillStyle = c
		ctx.fillRect(0, 0, 1, 1)
		const d = ctx.getImageData(0, 0, 1, 1).data
		return { r: d[0], g: d[1], b: d[2], a: d[3] / 255 }
	}

	// Source-over compositing. Keeping the partial alpha matters: nib stacks
	// translucent panels (a 9% link tint over a 7% glass panel over the shell),
	// and flattening each step to alpha 1 makes the topmost tint the background.
	function over(fg, bg) {
		const a = fg.a + bg.a * (1 - fg.a)
		if (a === 0) return { r: 0, g: 0, b: 0, a: 0 }
		return {
			r: (fg.r * fg.a + bg.r * bg.a * (1 - fg.a)) / a,
			g: (fg.g * fg.a + bg.g * bg.a * (1 - fg.a)) / a,
			b: (fg.b * fg.a + bg.b * bg.a * (1 - fg.a)) / a,
			a,
		}
	}

	function effectiveBg(el) {
		let acc = { r: 0, g: 0, b: 0, a: 0 }
		let node = el
		while (node) {
			const c = parse(getComputedStyle(node).backgroundColor)
			if (c && c.a > 0) {
				acc = over(acc, c)
				if (acc.a >= 0.999) return acc
			}
			node = node.parentElement
		}
		// Nothing opaque underneath: fall back to the canvas ground.
		return over(acc, { r: 255, g: 255, b: 255, a: 1 })
	}

	function luminance(c) {
		const f = (v) => {
			v /= 255
			return v <= 0.03928 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4)
		}
		return 0.2126 * f(c.r) + 0.7152 * f(c.g) + 0.0722 * f(c.b)
	}

	window.__measureContrast = (selectors) => {
		const out = []
		for (const selector of selectors) {
			for (const el of document.querySelectorAll(selector)) {
				const text = (el.textContent || '').trim()
				if (!text) continue
				const rect = el.getBoundingClientRect()
				if (rect.width === 0 || rect.height === 0) continue

				const style = getComputedStyle(el)
				const bg = effectiveBg(el)
				const colour = parse(style.color)
				if (!colour) continue
				const fg = over(colour, bg)
				const l1 = luminance(fg)
				const l2 = luminance(bg)
				const ratio = (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05)

				const size = parseFloat(style.fontSize)
				const weight = Number(style.fontWeight) || 400
				// WCAG "large text": 24px, or 18.66px when bold.
				const large = size >= 24 || (size >= 18.66 && weight >= 700)

				out.push({
					selector,
					text: text.slice(0, 40),
					ratio: Math.round(ratio * 100) / 100,
					size,
					weight,
					required: large ? 3 : 4.5,
				})
			}
		}
		return out
	}
})()
`

export interface ContrastSample {
	selector: string
	text: string
	ratio: number
	size: number
	weight: number
	required: number
}

export async function measureContrast(
	page: Page,
	selectors: string[],
): Promise<ContrastSample[]> {
	await page.addScriptTag({ content: SCRIPT })
	return page.evaluate(
		(sels) =>
			(window as unknown as {
				__measureContrast: (s: string[]) => ContrastSample[]
			}).__measureContrast(sels),
		selectors,
	)
}

export function failing(samples: ContrastSample[]): ContrastSample[] {
	return samples.filter((s) => s.ratio < s.required)
}
