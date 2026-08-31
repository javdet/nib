import {
	type ReactNode,
	useCallback,
	useEffect,
	useRef,
	useState,
} from 'react'
import { cn } from '@/lib/utils'
import { ChatPanel } from './chat-panel'

const DEFAULT_LEFT_PERCENT = 70
const MIN_LEFT_PERCENT = 20
const MAX_LEFT_PERCENT = 85

export function MainSplitLayout({ children }: { children: ReactNode }) {
	const [leftPercent, setLeftPercent] = useState(DEFAULT_LEFT_PERCENT)
	const [dragging, setDragging] = useState(false)
	const containerRef = useRef<HTMLDivElement>(null)

	const onMove = useCallback((e: MouseEvent) => {
		const el = containerRef.current
		if (!el) return
		const rect = el.getBoundingClientRect()
		const x = e.clientX - rect.left
		const pct = (x / rect.width) * 100
		setLeftPercent(
			Math.min(MAX_LEFT_PERCENT, Math.max(MIN_LEFT_PERCENT, pct)),
		)
	}, [])

	useEffect(() => {
		if (!dragging) return
		function onUp() {
			setDragging(false)
		}
		window.addEventListener('mousemove', onMove)
		window.addEventListener('mouseup', onUp)
		return () => {
			window.removeEventListener('mousemove', onMove)
			window.removeEventListener('mouseup', onUp)
		}
	}, [dragging, onMove])

	useEffect(() => {
		if (!dragging) return
		const prev = document.body.style.userSelect
		document.body.style.userSelect = 'none'
		return () => {
			document.body.style.userSelect = prev
		}
	}, [dragging])

	return (
		<div
			ref={containerRef}
			className="flex min-h-0 min-w-0 flex-1 flex-row overflow-hidden"
		>
			<main
				className="min-h-0 min-w-0 overflow-y-auto px-6 pb-6 pt-20"
				style={{ flex: `${leftPercent} 1 0%` }}
			>
				{children}
			</main>
			<div
				role="separator"
				aria-orientation="vertical"
				aria-valuemin={MIN_LEFT_PERCENT}
				aria-valuemax={MAX_LEFT_PERCENT}
				aria-valuenow={Math.round(leftPercent)}
				tabIndex={0}
				onMouseDown={() => setDragging(true)}
				onKeyDown={(e) => {
					if (e.key === 'ArrowLeft') {
						e.preventDefault()
						setLeftPercent((p) => Math.max(MIN_LEFT_PERCENT, p - 2))
					} else if (e.key === 'ArrowRight') {
						e.preventDefault()
						setLeftPercent((p) => Math.min(MAX_LEFT_PERCENT, p + 2))
					}
				}}
				className={cn(
					'group relative w-1.5 shrink-0 cursor-col-resize bg-border/50',
					'transition-colors duration-[var(--dur-fast)] focus-ring-inset',
					'hover:bg-ring/50',
					dragging && 'bg-ring/60',
				)}
			>
				<span className="sr-only">Resize panels</span>
			</div>
			<aside
				className="flex min-h-0 min-w-0 flex-col border-l border-border bg-background pt-14"
				style={{ flex: `${100 - leftPercent} 1 0%` }}
			>
				<ChatPanel />
			</aside>
		</div>
	)
}
