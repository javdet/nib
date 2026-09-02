import { useEffect, useId, useRef, useState } from 'react'
import {
	buildDagConfig,
	decorateDagSvg,
	loadDagFont,
	readDagPalette,
} from '../lib/dag-render'

function extractMermaidSource(content: string): string {
	let body = content.trim()
	body = body.replace(/^```mermaid\s*/i, '')
	body = body.replace(/^```\s*/, '')
	body = body.replace(/\s*```$/, '')
	return body.trim()
}

interface DagViewProps {
	content: string
}

export function DagView({ content }: DagViewProps) {
	const containerRef = useRef<HTMLDivElement>(null)
	const renderSeq = useRef(0)
	const renderId = useId().replace(/:/g, '')
	const [renderError, setRenderError] = useState<string | null>(null)

	useEffect(() => {
		const source = extractMermaidSource(content)
		if (!source) {
			setRenderError('Empty DAG content')
			if (containerRef.current) {
				containerRef.current.innerHTML = ''
			}
			return
		}

		let cancelled = false
		renderSeq.current += 1
		// The id prefix is what the board styles hook onto, including during
		// mermaid's own off-screen measurement pass.
		const diagramId = `dag-${renderId}-${renderSeq.current}`

		async function render() {
			try {
				const [mermaid] = await Promise.all([
					import('mermaid').then((module) => module.default),
					loadDagFont(),
				])
				if (cancelled) return

				const palette = readDagPalette(document.documentElement)
				mermaid.initialize(buildDagConfig(palette))
				const { svg } = await mermaid.render(diagramId, source)
				if (cancelled || !containerRef.current) return

				containerRef.current.innerHTML = svg
				const rendered = containerRef.current.querySelector('svg')
				if (rendered) {
					decorateDagSvg(rendered)
				}
				setRenderError(null)
			} catch (err) {
				if (cancelled) return
				if (containerRef.current) {
					containerRef.current.innerHTML = ''
				}
				setRenderError(
					err instanceof Error ? err.message : 'Failed to render DAG',
				)
			}
		}

		void render()
		return () => {
			cancelled = true
		}
	}, [content, renderId])

	return (
		<>
			{renderError ? (
				<div className="rounded-md border border-destructive/35 bg-destructive/12 px-4 py-3 text-sm text-destructive">
					{renderError}
				</div>
			) : null}
			{/* Hidden rather than unmounted on error: the next render needs the
			    container to still be there to draw into. */}
			<div
				ref={containerRef}
				className="overflow-x-auto py-2 text-center [&_svg]:mx-auto [&_svg]:inline-block"
				hidden={renderError !== null}
			/>
		</>
	)
}
