import * as React from 'react'
import * as DialogPrimitive from '@radix-ui/react-dialog'
import { X } from 'lucide-react'

import { cn } from '@/lib/utils'

const Dialog = DialogPrimitive.Root
const DialogTrigger = DialogPrimitive.Trigger
const DialogPortal = DialogPrimitive.Portal
const DialogClose = DialogPrimitive.Close

const DialogOverlay = React.forwardRef<
	React.ComponentRef<typeof DialogPrimitive.Overlay>,
	React.ComponentPropsWithoutRef<typeof DialogPrimitive.Overlay>
>(function DialogOverlay({ className, ...props }, ref) {
	return (
		<DialogPrimitive.Overlay
			ref={ref}
			className={cn(
				'scrim fixed inset-0 z-50 duration-[var(--dur-med)]',
				'data-[state=open]:animate-in data-[state=closed]:animate-out',
				'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
				className,
			)}
			{...props}
		/>
	)
})

const DialogContent = React.forwardRef<
	React.ComponentRef<typeof DialogPrimitive.Content>,
	React.ComponentPropsWithoutRef<typeof DialogPrimitive.Content>
>(function DialogContent({ className, children, ...props }, ref) {
	return (
		<DialogPortal>
			<DialogOverlay />
			<DialogPrimitive.Content
				ref={ref}
				className={cn(
					'fixed left-[50%] top-[50%] z-50 grid w-full max-w-lg gap-4',
					'translate-x-[-50%] translate-y-[-50%] rounded-xl border',
					'border-border/60 bg-background p-6 elev-3',
					'duration-[var(--dur-med)] ease-[var(--ease-out)]',
					'data-[state=open]:animate-in data-[state=closed]:animate-out',
					'data-[state=closed]:fade-out-0 data-[state=open]:fade-in-0',
					'data-[state=closed]:zoom-out-95 data-[state=open]:zoom-in-95',
					'data-[state=closed]:slide-out-to-left-1/2',
					'data-[state=closed]:slide-out-to-top-[48%]',
					'data-[state=open]:slide-in-from-left-1/2',
					'data-[state=open]:slide-in-from-top-[48%]',
					className,
				)}
				{...props}
			>
				{children}
				<DialogPrimitive.Close
						className={cn(
							'absolute right-3 top-3 flex size-8 cursor-pointer',
							'items-center justify-center rounded-md focus-ring',
							'text-muted-foreground transition-colors',
							'duration-[var(--dur-fast)] hover:bg-foreground/[0.07]',
							'hover:text-foreground disabled:pointer-events-none',
						)}
					>
					<X className="h-4 w-4" />
					<span className="sr-only">Close</span>
				</DialogPrimitive.Close>
			</DialogPrimitive.Content>
		</DialogPortal>
	)
})

function DialogHeader({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
	return (
		<div
			className={cn(
				// pr-6 keeps long titles clear of the close button
				'flex flex-col space-y-1.5 pr-6 text-center sm:text-left',
				className,
			)}
			{...props}
		/>
	)
}

function DialogFooter({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
	return (
		<div
			className={cn('flex flex-col-reverse sm:flex-row sm:justify-end sm:space-x-2', className)}
			{...props}
		/>
	)
}

const DialogTitle = React.forwardRef<
	React.ComponentRef<typeof DialogPrimitive.Title>,
	React.ComponentPropsWithoutRef<typeof DialogPrimitive.Title>
>(function DialogTitle({ className, ...props }, ref) {
	return (
		<DialogPrimitive.Title
			ref={ref}
			className={cn('text-lg font-semibold leading-none tracking-tight', className)}
			{...props}
		/>
	)
})

const DialogDescription = React.forwardRef<
	React.ComponentRef<typeof DialogPrimitive.Description>,
	React.ComponentPropsWithoutRef<typeof DialogPrimitive.Description>
>(function DialogDescription({ className, ...props }, ref) {
	return (
		<DialogPrimitive.Description
			ref={ref}
			className={cn('text-sm text-muted-foreground', className)}
			{...props}
		/>
	)
})

export {
	Dialog,
	DialogPortal,
	DialogOverlay,
	DialogTrigger,
	DialogClose,
	DialogContent,
	DialogHeader,
	DialogFooter,
	DialogTitle,
	DialogDescription,
}
