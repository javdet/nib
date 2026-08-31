import * as React from 'react'
import { ChevronDown } from 'lucide-react'

import { cn } from '@/lib/utils'
import { fieldClasses } from '@/components/ui/input'

interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
	/** Classes for the wrapper element rather than the <select> itself. */
	wrapperClassName?: string
}

/**
 * Native <select> with app chrome instead of OS chrome. Deliberately not a
 * Radix Select: keeping the native element means value/onChange/<option>
 * semantics at every call site stay exactly as they were. The popup list
 * itself is themed by `color-scheme` on the root element.
 */
const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
	function Select({ className, wrapperClassName, children, ...props }, ref) {
		return (
			<div className={cn('relative w-full', wrapperClassName)}>
				<select
					className={cn(
						fieldClasses,
						'h-[var(--control-h)] appearance-none py-1 pl-3 pr-9 text-sm',
						className,
					)}
					ref={ref}
					{...props}
				>
					{children}
				</select>
				<ChevronDown
					aria-hidden="true"
					className={cn(
						'pointer-events-none absolute right-3 top-1/2 h-4 w-4',
						'-translate-y-1/2 text-muted-foreground',
						props.disabled && 'opacity-50',
					)}
				/>
			</div>
		)
	},
)

export { Select }
