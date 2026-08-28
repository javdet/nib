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
				className="break-all rounded bg-background/60 px-1 py-0.5 font-mono text-[0.85em]"
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
				className="mr-1 align-middle"
			/>
		)
	},
}

interface MarkdownMessageProps {
	content: string
}

export function MarkdownMessage({ content }: MarkdownMessageProps) {
	return (
		<div className="min-w-0 break-words">
			<ReactMarkdown
				remarkPlugins={[remarkGfm, remarkBreaks]}
				components={markdownComponents}
			>
				{content}
			</ReactMarkdown>
		</div>
	)
}
