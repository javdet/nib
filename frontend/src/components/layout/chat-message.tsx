import { type ReactNode } from 'react'
import { Bot, RotateCcw, User } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { TableMessage } from '@/features/dialogs/components/table-message'
import { MarkdownMessage } from '@/components/markdown-message'
import type { Attachment } from '@/features/dialogs/api/dialogs'
import type { TableData } from '@/features/dialogs/api/dialogs'

export interface ChatDisplayMessage {
	id: string
	role: string
	content: string
	table?: TableData
	attachments?: Attachment[]
}

interface ChatMessageProps {
	message: ChatDisplayMessage
	showRetry?: boolean
	onRetry?: () => void
	renderAttachmentChip: (attachment: Attachment) => ReactNode
}

function renderMessageAttachments(
	attachments: ChatDisplayMessage['attachments'],
	renderAttachmentChip: ChatMessageProps['renderAttachmentChip'],
) {
	if (!attachments?.length) return null
	return (
		<div className="mt-1.5 flex flex-wrap gap-1.5">
			{attachments.map((a) => renderAttachmentChip(a))}
		</div>
	)
}

export function ChatMessage({
	message,
	showRetry,
	onRetry,
	renderAttachmentChip,
}: ChatMessageProps) {
	if (message.table) {
		return (
			<div>
				<TableMessage table={message.table} />
				{showRetry && onRetry ? (
					<div className="mt-2 flex justify-end pr-2">
						<Button
							type="button"
							variant="ghost"
							size="sm"
							className="h-7 gap-1.5 px-2 text-xs text-muted-foreground hover:text-foreground"
							onClick={() => void onRetry()}
						>
							<RotateCcw className="h-3 w-3" />
							Retry
						</Button>
					</div>
				) : null}
			</div>
		)
	}

	return (
		<div
			className={cn(
				'flex min-w-0 gap-2',
				message.role === 'user' ? 'flex-row-reverse' : 'flex-row',
			)}
		>
			<div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border bg-muted">
				{message.role === 'user' ? (
					<User className="h-3.5 w-3.5" />
				) : (
					<Bot className="h-3.5 w-3.5" />
				)}
			</div>
			<div
				className={cn(
					'min-w-0 max-w-[85%] overflow-hidden rounded-lg px-3 py-2 text-sm',
					message.role === 'user' && 'whitespace-pre-wrap break-words',
					message.role === 'user'
						? 'bg-primary text-primary-foreground'
						: 'bg-muted',
				)}
			>
				{message.role === 'assistant' && message.content ? (
					<MarkdownMessage content={message.content} />
				) : (
					message.content
				)}
				{renderMessageAttachments(message.attachments, renderAttachmentChip)}
			</div>
			{showRetry && onRetry ? (
				<Button
					type="button"
					variant="ghost"
					size="sm"
					className="h-7 gap-1.5 self-end px-2 text-xs text-muted-foreground hover:text-foreground"
					onClick={() => void onRetry()}
				>
					<RotateCcw className="h-3 w-3" />
					Retry
				</Button>
			) : null}
		</div>
	)
}
