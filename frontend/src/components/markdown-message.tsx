import { useMemo } from 'react'
import ReactMarkdown, { type Components } from 'react-markdown'
import remarkBreaks from 'remark-breaks'
import remarkGfm from 'remark-gfm'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import { cn } from '@/lib/utils'
import { dialogIdFromHref } from '@/lib/dialog-link'

const markdownComponents: Components = {
	p: ({ children }) => (
		<p className="mb-2 last:mb-0 leading-relaxed">{children}</p>
	),
	ul: ({ children }) => (
		<ul className="mb-2 list-disc space-y-1 pl-5 last:mb-0">{children}</ul>
	),
	ol: ({ children }) => (
		<ol className="mb-2 list-decimal space-y-1 pl-5 last:mb-0">
			{children}
		</ol>
	),
	li: ({ children }) => <li className="leading-relaxed">{children}</li>,
	h1: ({ children }) => (
		<h1 className="mt-3 mb-1 first:mt-0 text-base font-semibold">
			{children}
		</h1>
	),
	h2: ({ children }) => (
		<h2 className="mt-3 mb-1 first:mt-0 text-sm font-semibold">
			{children}
		</h2>
	),
	h3: ({ children }) => (
		<h3 className="mt-3 mb-1 first:mt-0 text-sm font-semibold">
			{children}
		</h3>
	),
	strong: ({ children }) => (
		<strong className="font-semibold">{children}</strong>
	),
	em: ({ children }) => <em className="italic">{children}</em>,
	del: ({ children }) => <del>{children}</del>,
	code: ({ className, children, ...props }) => {
		const isBlock = Boolean(className)
		if (isBlock) {
			return (
				<code className={cn('font-mono text-xs', className)} {...props}>
					{children}
				</code>
			)
		}
		return (
			<code
				className="break-all rounded bg-background/60 px-1 py-0.5 font-mono text-[0.85em] text-code"
				{...props}
			>
				{children}
			</code>
		)
	},
	pre: ({ children }) => (
		<pre className="mb-2 overflow-x-auto rounded-md border bg-background/60 p-2 text-xs last:mb-0">
			{children}
		</pre>
	),
	a: ({ href, children }) => (
		<a
			href={href}
			target="_blank"
			rel="noreferrer noopener"
			className="break-all underline underline-offset-2"
		>
			{children}
		</a>
	),
	blockquote: ({ children }) => (
		<blockquote className="mb-2 border-l-2 border-border pl-3 text-muted-foreground last:mb-0">
			{children}
		</blockquote>
	),
	hr: () => <hr className="my-3 border-border" />,
	table: ({ children }) => (
		<div className="mb-2 overflow-x-auto last:mb-0">
			<Table>{children}</Table>
		</div>
	),
	thead: ({ children }) => <TableHeader>{children}</TableHeader>,
	tbody: ({ children }) => <TableBody>{children}</TableBody>,
	tr: ({ children }) => <TableRow>{children}</TableRow>,
	th: ({ children }) => (
		<TableHead className="h-8 whitespace-nowrap px-2 text-xs font-medium">
			{children}
		</TableHead>
	),
	td: ({ children }) => (
		<TableCell className="whitespace-pre-line px-2 py-1.5 align-top text-xs">
			{children}
		</TableCell>
	),
	input: ({ type, checked, disabled }) => {
		if (type !== 'checkbox') {
			return null
		}
		return (
			<input
				type="checkbox"
				checked={checked}
				disabled={disabled ?? true}
				readOnly
				className="mr-1.5 size-3.5 align-middle accent-[var(--primary)]"
			/>
		)
	},
}

function buildMarkdownComponents(
	onDialogLink?: (dialogId: string) => void,
): Components {
	return {
	...markdownComponents,
	a: ({ href, children }) => {
		// There is no route to a dialog by id -- the app switches dialogs through
		// context -- so a link to one has to be intercepted rather than followed.
		const dialogId = dialogIdFromHref(href)
		if (dialogId && onDialogLink) {
			return (
				<button
					type="button"
					onClick={() => onDialogLink(dialogId)}
					className="cursor-pointer underline underline-offset-2"
				>
					{children}
				</button>
			)
		}
		if (dialogId) {
			// Nothing can open it from here; showing a dead link would be worse.
			return <span>{children}</span>
		}
		return (
			<a
				href={href}
				target="_blank"
				rel="noreferrer noopener"
				className="break-all underline underline-offset-2"
			>
				{children}
			</a>
		)
	},
	}
}


interface MarkdownMessageProps {
	content: string
	/**
	 * Called instead of navigating when the message links to another dialog.
	 * Omitted everywhere the surrounding view cannot switch dialogs, which
	 * renders such a link as plain text rather than a link that goes nowhere.
	 */
	onDialogLink?: (dialogId: string) => void
}

export function MarkdownMessage({
	content,
	onDialogLink,
}: MarkdownMessageProps) {
	const components = useMemo(
		() => buildMarkdownComponents(onDialogLink),
		[onDialogLink],
	)

	return (
		<div className="min-w-0 break-words">
			<ReactMarkdown
				remarkPlugins={[remarkGfm, remarkBreaks]}
				components={components}
			>
				{content}
			</ReactMarkdown>
		</div>
	)
}
