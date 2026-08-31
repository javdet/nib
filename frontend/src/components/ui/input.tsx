import * as React from 'react'

import { cn } from '@/lib/utils'

/**
 * Shared field surface. Kept in one place so Input, Textarea and Select stay
 * visually identical.
 */
const fieldClasses = cn(
	'w-full rounded-md border border-input bg-background/40 text-foreground',
	'elev-1 transition-[background-color,border-color,box-shadow]',
	'duration-[var(--dur-fast)] ease-[var(--ease-standard)]',
	'placeholder:text-muted-foreground/70',
	'hover:border-ring/40',
	'focus-ring focus-visible:border-ring/60',
	'aria-[invalid=true]:border-destructive/70',
	'disabled:cursor-not-allowed disabled:bg-muted/30 disabled:opacity-60',
)

const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
	function Input({ className, type, ...props }, ref) {
		return (
			<input
				type={type}
				className={cn(
					fieldClasses,
					'flex h-[var(--control-h)] px-3 py-1 text-base md:text-sm',
					'file:mr-3 file:h-7 file:cursor-pointer file:rounded-md file:border-0',
					'file:bg-secondary file:px-3 file:text-sm file:font-medium',
					'file:text-secondary-foreground hover:file:bg-secondary/80',
					className,
				)}
				ref={ref}
				{...props}
			/>
		)
	},
)

export { Input, fieldClasses }
