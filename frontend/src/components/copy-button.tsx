import { useCallback, useEffect, useRef, useState } from 'react'
import { Check, Copy } from 'lucide-react'

import { IconButton } from '@/components/ui/icon-button'
import { cn } from '@/lib/utils'

interface CopyButtonProps {
	text: string
	className?: string
	label?: string
}

export function CopyButton({
	text,
	className,
	label = 'Copy message',
}: CopyButtonProps) {
	const [copied, setCopied] = useState(false)
	const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null)

	useEffect(() => {
		return () => {
			if (timeoutRef.current) {
				clearTimeout(timeoutRef.current)
			}
		}
	}, [])

	const handleCopy = useCallback(async () => {
		try {
			await navigator.clipboard.writeText(text)
			setCopied(true)
			if (timeoutRef.current) {
				clearTimeout(timeoutRef.current)
			}
			timeoutRef.current = setTimeout(() => {
				setCopied(false)
				timeoutRef.current = null
			}, 1500)
		} catch {
			// Clipboard access may be denied; leave icon unchanged.
		}
	}, [text])

	return (
		<IconButton
			type="button"
			variant="ghost"
			className={cn(
				'h-7 w-7 text-muted-foreground hover:text-foreground',
				className,
			)}
			onClick={() => void handleCopy()}
			tooltip={copied ? 'Copied' : label}
		>
			{copied ? (
				<Check className="h-3.5 w-3.5" />
			) : (
				<Copy className="h-3.5 w-3.5" />
			)}
		</IconButton>
	)
}
