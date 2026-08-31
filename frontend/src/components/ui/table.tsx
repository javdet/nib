import * as React from 'react'

import { cn } from '@/lib/utils'

const Table = React.forwardRef<HTMLTableElement, React.HTMLAttributes<HTMLTableElement>>(
	function Table({ className, ...props }, ref) {
		return (
			<div className="relative w-full overflow-auto">
				<table
					ref={ref}
					className={cn('w-full caption-bottom text-sm tabular', className)}
					{...props}
				/>
			</div>
		)
	},
)

const TableHeader = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(
	function TableHeader({ className, ...props }, ref) {
		return <thead ref={ref} className={cn('[&_tr]:border-b [&_tr]:border-border/60', className)} {...props} />
	},
)

const TableBody = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(
	function TableBody({ className, ...props }, ref) {
		return (
			<tbody
				ref={ref}
				className={cn('[&_tr:last-child]:border-0', className)}
				{...props}
			/>
		)
	},
)

const TableFooter = React.forwardRef<HTMLTableSectionElement, React.HTMLAttributes<HTMLTableSectionElement>>(
	function TableFooter({ className, ...props }, ref) {
		return (
			<tfoot
				ref={ref}
				className={cn(
					'border-t bg-muted/50 font-medium [&>tr]:last:border-b-0',
					className,
				)}
				{...props}
			/>
		)
	},
)

const TableRow = React.forwardRef<HTMLTableRowElement, React.HTMLAttributes<HTMLTableRowElement>>(
	function TableRow({ className, ...props }, ref) {
		return (
			<tr
				ref={ref}
				className={cn(
					'border-b border-border/50 transition-colors',
					'duration-[var(--dur-fast)] hover:bg-foreground/[0.035]',
					'data-[state=selected]:bg-foreground/[0.06]',
					className,
				)}
				{...props}
			/>
		)
	},
)

const TableHead = React.forwardRef<HTMLTableCellElement, React.ThHTMLAttributes<HTMLTableCellElement>>(
	function TableHead({ className, ...props }, ref) {
		return (
			<th
				ref={ref}
				className={cn(
					'h-10 px-2 text-left align-middle text-xs font-semibold uppercase',
					'tracking-wider text-muted-foreground',
					'[&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]',
					className,
				)}
				{...props}
			/>
		)
	},
)

const TableCell = React.forwardRef<HTMLTableCellElement, React.TdHTMLAttributes<HTMLTableCellElement>>(
	function TableCell({ className, ...props }, ref) {
		return (
			<td
				ref={ref}
				className={cn(
					'px-2 py-2.5 align-middle',
					'[&:has([role=checkbox])]:pr-0 [&>[role=checkbox]]:translate-y-[2px]',
					className,
				)}
				{...props}
			/>
		)
	},
)

const TableCaption = React.forwardRef<HTMLTableCaptionElement, React.HTMLAttributes<HTMLTableCaptionElement>>(
	function TableCaption({ className, ...props }, ref) {
		return (
			<caption
				ref={ref}
				className={cn('mt-4 text-sm text-muted-foreground', className)}
				{...props}
			/>
		)
	},
)

export {
	Table,
	TableHeader,
	TableBody,
	TableFooter,
	TableHead,
	TableRow,
	TableCell,
	TableCaption,
}
