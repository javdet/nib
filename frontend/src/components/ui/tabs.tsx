import * as React from 'react'
import * as TabsPrimitive from '@radix-ui/react-tabs'

import { cn } from '@/lib/utils'

const Tabs = TabsPrimitive.Root

const TabsList = React.forwardRef<
	React.ElementRef<typeof TabsPrimitive.List>,
	React.ComponentPropsWithoutRef<typeof TabsPrimitive.List>
>(function TabsList({ className, ...props }, ref) {
	return (
		<TabsPrimitive.List
			ref={ref}
			className={cn(
				'inline-flex h-[var(--control-h)] items-center justify-center gap-1',
				'rounded-lg border border-border/50 bg-muted/40 p-1',
				'text-muted-foreground',
				className,
			)}
			{...props}
		/>
	)
})

const TabsTrigger = React.forwardRef<
	React.ElementRef<typeof TabsPrimitive.Trigger>,
	React.ComponentPropsWithoutRef<typeof TabsPrimitive.Trigger>
>(function TabsTrigger({ className, ...props }, ref) {
	return (
		<TabsPrimitive.Trigger
			ref={ref}
			className={cn(
				'inline-flex cursor-pointer items-center justify-center gap-2',
				'whitespace-nowrap rounded-md px-3 py-1 text-sm font-medium',
				'transition-[background-color,color,box-shadow]',
				'duration-[var(--dur-fast)] ease-[var(--ease-standard)] focus-ring',
				'hover:text-foreground',
				'disabled:pointer-events-none disabled:opacity-50',
				'data-[state=active]:bg-background data-[state=active]:text-foreground',
				'data-[state=active]:elev-1',
				className,
			)}
			{...props}
		/>
	)
})

const TabsContent = React.forwardRef<
	React.ElementRef<typeof TabsPrimitive.Content>,
	React.ComponentPropsWithoutRef<typeof TabsPrimitive.Content>
>(function TabsContent({ className, ...props }, ref) {
	return (
		<TabsPrimitive.Content
			ref={ref}
			className={cn(
				'mt-4 focus-ring',
				className,
			)}
			{...props}
		/>
	)
})

export { Tabs, TabsList, TabsTrigger, TabsContent }
