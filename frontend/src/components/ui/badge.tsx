import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'

import { cn } from '@/lib/utils'

const badgeVariants = cva(
	cn(
		'inline-flex items-center gap-1.5 rounded-md border px-2.5 py-0.5',
		'text-xs font-semibold leading-5 transition-colors',
		'duration-[var(--dur-fast)] focus-ring',
	),
	{
		variants: {
			variant: {
				default:
					'border-transparent bg-primary text-primary-foreground elev-1 hover:bg-primary/85',
				secondary:
					'border-border/60 bg-secondary text-secondary-foreground hover:bg-secondary/80',
				destructive:
					'border-destructive/30 bg-destructive/12 text-destructive hover:bg-destructive/20',
				outline: 'border-border bg-background/40 text-foreground',
			},
		},
		defaultVariants: {
			variant: 'default',
		},
	},
)

interface BadgeProps
	extends React.HTMLAttributes<HTMLDivElement>,
		VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
	return (
		<div className={cn(badgeVariants({ variant }), className)} {...props} />
	)
}

export { Badge, badgeVariants }
export type { BadgeProps }
