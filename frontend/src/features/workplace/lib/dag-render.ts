import type { MermaidConfig } from 'mermaid'

/**
 * Fallbacks for the lettering tokens in index.css. Both reach mermaid, which
 * sizes every node from a measurement pass: it has to lay the labels out in
 * the very font and size they are later drawn with.
 */
const DAG_FONT_FAMILY = "'Neucha', 'Bradley Hand', 'Segoe Print', cursive"
const DAG_FONT_SIZE = '22px'

/** How much bigger than mermaid's default the arrowheads are drawn. */
const ARROW_SCALE = 1.7

/**
 * Probe text for the font load. It has to span both scripts: fonts.load()
 * only fetches the @font-face whose unicode-range covers the characters it is
 * handed, and its default probe is latin — which left cyrillic labels measured
 * in a fallback font and boxes sized for text that never appeared.
 */
const FONT_PROBE = 'Aa Яя'

/** Marker colours available to the board, matching --dag-pen-N in index.css. */
const PEN_COUNT = 5

/** Node rows closer together than this belong to the same rank. */
const RANK_TOLERANCE = 24

const TRANSLATE_PATTERN = /translate\(\s*(-?[\d.]+)[\s,]+(-?[\d.]+)/

export interface DagPalette {
	ink: string
	line: string
	nodeFill: string
	clusterStroke: string
	fontFamily: string
	fontSize: string
}

function readToken(
	styles: CSSStyleDeclaration,
	name: string,
	fallback: string,
): string {
	return styles.getPropertyValue(name).trim() || fallback
}

/**
 * DAG colours are declared as hex in index.css because they end up here:
 * mermaid derives shades with khroma, which cannot parse the oklch() the rest
 * of the theme is written in.
 */
export function readDagPalette(element: Element): DagPalette {
	const styles = getComputedStyle(element)
	return {
		ink: readToken(styles, '--dag-ink', '#2b3242'),
		line: readToken(styles, '--dag-line', '#58627a'),
		nodeFill: readToken(styles, '--dag-node-fill', '#ffffff'),
		clusterStroke: readToken(styles, '--dag-cluster-stroke', '#7c8296'),
		fontFamily: readToken(styles, '--dag-font-family', DAG_FONT_FAMILY),
		fontSize: readToken(styles, '--dag-font-size', DAG_FONT_SIZE),
	}
}

export function buildDagConfig(palette: DagPalette): MermaidConfig {
	return {
		startOnLoad: false,
		securityLevel: 'strict',
		theme: 'base',
		look: 'handDrawn',
		// Fixed seed so redrawing unchanged content does not reshuffle every
		// sketch line.
		handDrawnSeed: 42,
		fontFamily: palette.fontFamily,
		flowchart: {
			curve: 'basis',
			htmlLabels: true,
			padding: 12,
			nodeSpacing: 56,
			rankSpacing: 66,
			useMaxWidth: true,
		},
		themeVariables: {
			fontFamily: palette.fontFamily,
			fontSize: palette.fontSize,
			background: palette.nodeFill,
			primaryColor: palette.nodeFill,
			primaryBorderColor: palette.line,
			primaryTextColor: palette.ink,
			secondaryColor: palette.nodeFill,
			tertiaryColor: palette.nodeFill,
			mainBkg: palette.nodeFill,
			nodeBorder: palette.line,
			nodeTextColor: palette.ink,
			textColor: palette.ink,
			lineColor: palette.line,
			clusterBkg: palette.nodeFill,
			clusterBorder: palette.clusterStroke,
			edgeLabelBackground: palette.nodeFill,
		},
	}
}

/** Loads the hand font up front — mermaid measures labels with whatever is
 * available at render time, and a fallback typeface leaves boxes the wrong
 * size for the text finally drawn in them. */
export async function loadDagFont(): Promise<void> {
	if (typeof document === 'undefined' || !document.fonts) return
	try {
		await document.fonts.load(`400 ${DAG_FONT_SIZE} 'Neucha'`, FONT_PROBE)
		await document.fonts.ready
	} catch {
		// Fall through: the CSS stack still has system handwriting fonts.
	}
}

function rankOf(node: SVGGElement): number {
	const match = TRANSLATE_PATTERN.exec(node.getAttribute('transform') ?? '')
	return match ? Math.round(Number(match[2]) / RANK_TOLERANCE) : 0
}

/** Groups nodes by the row they were laid out on, top row first. */
function rankNodes(nodes: SVGGElement[]): number[] {
	const rows = nodes.map(rankOf)
	const ordered = [...new Set(rows)].sort((a, b) => a - b)
	return rows.map((row) => ordered.indexOf(row))
}

/**
 * Turbulence laid over rough.js' own sketching: it bends the long straight
 * runs and softens the corners that still give a machine away. The filter
 * goes into the diagram it belongs to and is applied inline, because an
 * element whose filter reference does not resolve is not painted at all —
 * and a stylesheet rule would already be pointing at this one while mermaid
 * measures the diagram, long before it exists.
 */
function addSketchFilter(svg: SVGSVGElement): string {
	const id = `${svg.id}-sketch`
	const ns = 'http://www.w3.org/2000/svg'
	const defs = document.createElementNS(ns, 'defs')
	defs.innerHTML =
		`<filter id="${id}" filterUnits="objectBoundingBox" ` +
		`x="-10%" y="-10%" width="120%" height="120%">` +
		`<feTurbulence type="fractalNoise" baseFrequency="0.017" ` +
		`numOctaves="2" seed="7" result="noise" />` +
		`<feDisplacementMap in="SourceGraphic" in2="noise" scale="3.6" ` +
		`xChannelSelector="R" yChannelSelector="G" />` +
		`</filter>`
	svg.insertBefore(defs, svg.firstChild)
	return `url(#${id})`
}

/**
 * Mermaid draws 8px arrowheads, which disappear next to 2px marker strokes.
 * Every marker is scaled up — refX/refY are in viewBox units, so the tip stays
 * on the line end — and links drawn without an arrow at all (`A --- B`) are
 * given the end marker too, since direction is the whole point of a DAG.
 */
function strengthenArrows(svg: SVGSVGElement): void {
	let pointEnd = ''

	svg.querySelectorAll('marker').forEach((marker) => {
		const width = Number(marker.getAttribute('markerWidth'))
		const height = Number(marker.getAttribute('markerHeight'))
		if (width) {
			marker.setAttribute('markerWidth', String(width * ARROW_SCALE))
		}
		if (height) {
			marker.setAttribute('markerHeight', String(height * ARROW_SCALE))
		}
		if (marker.id.endsWith('pointEnd')) {
			pointEnd = `url(#${marker.id})`
		}
	})

	if (!pointEnd) return
	svg.querySelectorAll('.edgePaths path').forEach((path) => {
		if (!path.getAttribute('marker-end')) {
			path.setAttribute('marker-end', pointEnd)
		}
	})
}

/**
 * rough.js emits a shape as a fill sketch followed by the outline stroke, with
 * nothing in the markup to tell them apart. Tagging them by position gives the
 * stylesheet a handle on each: the board drops the hachure and fills the
 * outline itself, which is what keeps the glass showing through.
 */
function tagRoughPaths(root: Element): void {
	root.querySelectorAll('g').forEach((group) => {
		const paths = Array.from(group.querySelectorAll(':scope > path'))
		paths.forEach((path, index) => {
			path.classList.add(
				index === paths.length - 1 ? 'dag-ink-stroke' : 'dag-ink-fill',
			)
		})
	})
}

/**
 * Turns a freshly rendered mermaid SVG into board drawing: one marker colour
 * per rank, and the stagger the CSS animations draw along.
 */
export function decorateDagSvg(svg: SVGSVGElement): void {
	const sketch = addSketchFilter(svg)
	strengthenArrows(svg)

	// handDrawn nodes come through as g.rough-node, plain ones as g.node.
	const nodes = Array.from(svg.querySelectorAll<SVGGElement>('.nodes > g'))
	const ranks = rankNodes(nodes)

	nodes.forEach((node, index) => {
		const rank = ranks[index] ?? 0
		node.classList.add(`dag-pen-${(rank % PEN_COUNT) + 1}`)
		node.style.setProperty('--dag-i', String(rank))
		tagRoughPaths(node)
	})

	svg.querySelectorAll<SVGGElement>('.clusters > g').forEach(tagRoughPaths)

	// Shape outlines only. Text inside a filtered group comes out smeared,
	// and a filtered link loses its arrowhead — Chrome drops the markers of
	// anything it renders through a filter. The links keep rough.js' own
	// waviness instead.
	svg.querySelectorAll<SVGElement>('.dag-ink-stroke').forEach((element) => {
		element.style.filter = sketch
	})

	// pathLength normalises every edge to a length of 1, which lets a single
	// dash keyframe draw links of any shape.
	svg.querySelectorAll<SVGPathElement>('.edgePaths path').forEach((path, i) => {
		path.setAttribute('pathLength', '1')
		path.style.setProperty('--dag-i', String(i))
	})

	svg
		.querySelectorAll<SVGGElement>('.edgeLabels > g')
		.forEach((label, i) => label.style.setProperty('--dag-i', String(i)))
}
