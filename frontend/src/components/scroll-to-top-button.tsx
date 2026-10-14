import { useEffect, useRef, useState } from 'react'
import { ArrowUp } from 'lucide-react'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'
import { scrollProgress, showScrollTop } from '@/lib/scroll-top'
import { cn } from '@/lib/utils'

const RING_RADIUS = 18
const RING_CIRCUMFERENCE = 2 * Math.PI * RING_RADIUS

function findScrollParent(el: HTMLElement | null): HTMLElement | null {
	for (let node = el?.parentElement; node; node = node.parentElement) {
		const { overflowY } = getComputedStyle(node)
		if (overflowY === 'auto' || overflowY === 'scroll') return node
	}
	return null
}

// Floating "back to top" control for long pages. It is placed as the last
// child of the page and pins itself to the bottom-right of whichever pane
// scrolls (the layout's <main>, not the window) with a zero-height sticky
// anchor, so it follows the split-pane divider and takes no layout space.
export function ScrollToTopButton({ className }: { className?: string }) {
	const anchorRef = useRef<HTMLDivElement>(null)
	const scrollerRef = useRef<HTMLElement | null>(null)
	const [visible, setVisible] = useState(false)
	const [progress, setProgress] = useState(0)

	useEffect(() => {
		const scroller = findScrollParent(anchorRef.current)
		scrollerRef.current = scroller
		if (!scroller) return

		// One measurement per frame: scroll fires far more often than paint.
		let frame = 0
		const measure = () => {
			frame = 0
			const metrics = {
				scrollTop: scroller.scrollTop,
				scrollHeight: scroller.scrollHeight,
				clientHeight: scroller.clientHeight,
			}
			setVisible(showScrollTop(metrics))
			setProgress(scrollProgress(metrics))
		}
		const handleScroll = () => {
			if (!frame) frame = requestAnimationFrame(measure)
		}

		measure()
		scroller.addEventListener('scroll', handleScroll, { passive: true })
		return () => {
			scroller.removeEventListener('scroll', handleScroll)
			if (frame) cancelAnimationFrame(frame)
		}
	}, [])

	function handleClick() {
		const reduceMotion = window.matchMedia(
			'(prefers-reduced-motion: reduce)',
		).matches
		scrollerRef.current?.scrollTo({
			top: 0,
			behavior: reduceMotion ? 'auto' : 'smooth',
		})
	}

	return (
		<div
			ref={anchorRef}
			className={cn(
				'pointer-events-none sticky bottom-6 z-10 h-0',
				className,
			)}
		>
			<div
				className={cn(
					'absolute right-0 bottom-0',
					'transition-[opacity,translate,scale] duration-[var(--dur-slow)] ease-[var(--ease-out)]',
					visible
						? 'translate-y-0 scale-100 opacity-100'
						: 'translate-y-3 scale-90 opacity-0',
				)}
			>
				<Tooltip>
					<TooltipTrigger asChild>
						<button
							type="button"
							onClick={handleClick}
							aria-label="Back to top"
							aria-hidden={!visible}
							tabIndex={visible ? 0 : -1}
							className={cn(
								'glass-menu group relative grid size-11 place-items-center rounded-full border',
								'text-foreground/75 hover:text-foreground focus-ring',
								'transition-[color,scale] duration-[var(--dur-med)] ease-[var(--ease-out)]',
								'hover:scale-105 active:scale-95',
								visible ? 'pointer-events-auto' : 'pointer-events-none',
							)}
						>
							<svg
								viewBox="0 0 44 44"
								className="absolute inset-0 size-full -rotate-90"
								aria-hidden
							>
								<circle
									cx="22"
									cy="22"
									r={RING_RADIUS}
									fill="none"
									strokeWidth="2"
									className="stroke-foreground/10"
								/>
								<circle
									cx="22"
									cy="22"
									r={RING_RADIUS}
									fill="none"
									strokeWidth="2"
									strokeLinecap="round"
									strokeDasharray={RING_CIRCUMFERENCE}
									strokeDashoffset={RING_CIRCUMFERENCE * (1 - progress)}
									className="stroke-current opacity-70"
								/>
							</svg>
							<ArrowUp
								className={cn(
									'relative size-4',
									'transition-transform duration-[var(--dur-med)] ease-[var(--ease-out)]',
									'group-hover:-translate-y-0.5',
								)}
								strokeWidth={2.25}
							/>
						</button>
					</TooltipTrigger>
					<TooltipContent side="left">Back to top</TooltipContent>
				</Tooltip>
			</div>
		</div>
	)
}
