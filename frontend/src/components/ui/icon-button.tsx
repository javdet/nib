import * as React from 'react'

import { Button, type ButtonProps } from '@/components/ui/button'
import {
	Tooltip,
	TooltipContent,
	TooltipTrigger,
} from '@/components/ui/tooltip'

interface IconButtonProps extends ButtonProps {
	tooltip: React.ReactNode
	tooltipSide?: React.ComponentProps<typeof TooltipContent>['side']
	tooltipDelay?: number
}

const IconButton = React.forwardRef<HTMLButtonElement, IconButtonProps>(
	function IconButton(
		{
			tooltip,
			tooltipSide = 'top',
			tooltipDelay,
			'aria-label': ariaLabel,
			disabled,
			size = 'icon',
			...props
		},
		ref,
	) {
		const resolvedAriaLabel =
			ariaLabel ?? (typeof tooltip === 'string' ? tooltip : undefined)

		const button = (
			<Button
				ref={ref}
				size={size}
				aria-label={resolvedAriaLabel}
				disabled={disabled}
				{...props}
			/>
		)

		const trigger = disabled ? (
			<span className="inline-flex" tabIndex={0}>
				{button}
			</span>
		) : (
			button
		)

		return (
			<Tooltip delayDuration={tooltipDelay}>
				<TooltipTrigger asChild>{trigger}</TooltipTrigger>
				<TooltipContent side={tooltipSide}>{tooltip}</TooltipContent>
			</Tooltip>
		)
	},
)

export { IconButton }
export type { IconButtonProps }
