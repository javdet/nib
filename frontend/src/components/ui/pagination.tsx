import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

interface PaginationProps {
	page: number
	total: number
	pageSize: number
	onPageChange: (page: number) => void
	className?: string
}

function getPageNumbers(current: number, totalPages: number): number[] {
	if (totalPages <= 7) {
		return Array.from({ length: totalPages }, (_, i) => i + 1)
	}

	const pages = new Set<number>()
	pages.add(1)
	pages.add(totalPages)

	for (let i = current - 2; i <= current + 2; i++) {
		if (i >= 1 && i <= totalPages) {
			pages.add(i)
		}
	}

	return Array.from(pages).sort((a, b) => a - b)
}

export function Pagination({
	page,
	total,
	pageSize,
	onPageChange,
	className,
}: PaginationProps) {
	const totalPages = Math.ceil(total / pageSize)

	if (totalPages <= 1) {
		return null
	}

	const pageNumbers = getPageNumbers(page, totalPages)

	return (
		<div
			className={cn(
				'flex items-center justify-center gap-1 pt-4',
				className,
			)}
		>
			<Button
				variant="outline"
				size="sm"
				onClick={() => onPageChange(page - 1)}
				disabled={page <= 1}
				aria-label="Previous page"
			>
				<ChevronLeft className="h-4 w-4" />
			</Button>

			{pageNumbers.map((pageNum, idx) => {
				const prev = pageNumbers[idx - 1]
				const showEllipsis = prev !== undefined && pageNum - prev > 1

				return (
					<span key={pageNum} className="flex items-center gap-1">
						{showEllipsis && (
							<span className="px-1 text-sm text-muted-foreground">
								…
							</span>
						)}
						<Button
							variant={pageNum === page ? 'default' : 'outline'}
							size="sm"
							className="min-w-9 tabular"
							onClick={() => onPageChange(pageNum)}
							aria-label={`Page ${pageNum}`}
							aria-current={pageNum === page ? 'page' : undefined}
						>
							{pageNum}
						</Button>
					</span>
				)
			})}

			<Button
				variant="outline"
				size="sm"
				onClick={() => onPageChange(page + 1)}
				disabled={page >= totalPages}
				aria-label="Next page"
			>
				<ChevronRight className="h-4 w-4" />
			</Button>
		</div>
	)
}
