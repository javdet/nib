import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const buttonVariants = cva(
	cn(
		'inline-flex cursor-pointer select-none items-center justify-center gap-2',
		'whitespace-nowrap rounded-md text-sm font-medium',
		'transition-[background-color,border-color,color,box-shadow,transform]',
		'duration-[var(--dur-fast)] ease-[var(--ease-standard)]',
		'focus-ring active:translate-y-px',
		'disabled:pointer-events-none disabled:opacity-50 disabled:shadow-none',
		'disabled:active:translate-y-0',
		'[&_svg]:pointer-events-none [&_svg]:size-4 [&_svg]:shrink-0',
	),
	{
		variants: {
			variant: {
				default: cn(
					'bg-primary text-primary-foreground elev-1',
					'hover:bg-primary/90 hover:elev-2',
				),
				destructive: cn(
					'border border-destructive/35 bg-destructive/12',
					'text-destructive elev-1',
					'hover:border-destructive/55 hover:bg-destructive/20',
				),
				outline: cn(
					'border border-input bg-background/40 elev-1',
					'hover:border-ring/40 hover:bg-accent hover:text-accent-foreground',
				),
				secondary: cn(
					'bg-secondary text-secondary-foreground elev-1',
					'hover:bg-secondary/80 hover:elev-2',
				),
				ghost: 'hover:bg-accent hover:text-accent-foreground',
				link: cn(
					'text-primary underline-offset-4',
					'hover:underline active:translate-y-0',
				),
			},
			size: {
				default: 'h-[var(--control-h)] px-4 py-2',
				sm: 'h-[var(--control-h-sm)] rounded-md px-3 text-xs',
				lg: 'h-[var(--control-h-lg)] rounded-md px-8',
				icon: 'h-[var(--control-h)] w-[var(--control-h)]',
				'icon-sm': 'h-[var(--control-h-sm)] w-[var(--control-h-sm)] rounded-md',
			},
		},
		defaultVariants: {
			variant: 'default',
			size: 'default',
		},
	},
)

interface ButtonProps
	extends React.ButtonHTMLAttributes<HTMLButtonElement>,
		VariantProps<typeof buttonVariants> {
	asChild?: boolean
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
	function Button({ className, variant, size, asChild = false, ...props }, ref) {
		const Comp = asChild ? Slot : 'button'
		return (
			<Comp
				className={cn(buttonVariants({ variant, size, className }))}
				ref={ref}
				{...props}
			/>
		)
	},
)

export { Button, buttonVariants }
export type { ButtonProps }
