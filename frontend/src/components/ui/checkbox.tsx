import * as React from 'react'
import * as CheckboxPrimitive from '@radix-ui/react-checkbox'
import { Check, Minus } from 'lucide-react'

import { cn } from '@/lib/utils'

const Checkbox = React.forwardRef<
	React.ElementRef<typeof CheckboxPrimitive.Root>,
	React.ComponentPropsWithoutRef<typeof CheckboxPrimitive.Root>
>(function Checkbox({ className, ...props }, ref) {
	return (
		<CheckboxPrimitive.Root
			ref={ref}
			className={cn(
				'peer size-4 shrink-0 cursor-pointer rounded-[5px] border border-input',
				'bg-background/40 elev-1',
				'transition-[background-color,border-color,box-shadow]',
				'duration-[var(--dur-fast)] ease-[var(--ease-standard)]',
				'hover:border-ring/60 focus-ring',
				'disabled:cursor-not-allowed disabled:opacity-50',
				'data-[state=checked]:border-primary data-[state=checked]:bg-primary',
				'data-[state=checked]:text-primary-foreground',
				'data-[state=indeterminate]:border-primary',
				'data-[state=indeterminate]:bg-primary',
				'data-[state=indeterminate]:text-primary-foreground',
				className,
			)}
			{...props}
		>
			<CheckboxPrimitive.Indicator
				className={cn(
					'flex items-center justify-center text-current',
					'animate-in fade-in-0 zoom-in-75 duration-[var(--dur-fast)]',
				)}
			>
				{props.checked === 'indeterminate' ? (
					<Minus className="size-3 stroke-[3]" />
				) : (
					<Check className="size-3 stroke-[3]" />
				)}
			</CheckboxPrimitive.Indicator>
		</CheckboxPrimitive.Root>
	)
})

export { Checkbox }
