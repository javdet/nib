import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Loader2, RefreshCw } from 'lucide-react'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { getActionContainerLogs } from '@/features/dialogs/api/dialogs'
import { ApiError } from '@/lib/api-client'

const REFRESH_MS = 10_000

// A refusal the endpoint will keep repeating: the run is over, its container is
// gone, or the executor is no longer one whose logs can be read. Polling past
// one of these only repeats the same message every ten seconds.
const TERMINAL_STATUSES = new Set([404, 409, 501])

interface ContainerLogsDialogProps {
	dialogId: string
	/** The action row key whose logs are shown, or null when closed. */
	actionKey: string | null
	number?: string
	onClose: () => void
}

/**
 * The live output of the agent-runner container behind a running code action.
 *
 * The body is a child of DialogContent on purpose: Radix unmounts that subtree
 * on close, so the log text -- which is never persisted anywhere -- is
 * discarded with it, and the polling effect's cleanup is the only teardown
 * path there is.
 */
export function ContainerLogsDialog({
	dialogId,
	actionKey,
	number,
	onClose,
}: ContainerLogsDialogProps) {
	return (
		<Dialog
			open={actionKey !== null}
			onOpenChange={(open) => {
				if (!open) onClose()
			}}
		>
			<DialogContent className="max-w-3xl">
				<DialogHeader>
					<DialogTitle>
						Container logs{number ? ` — ${number}` : ''}
					</DialogTitle>
					<DialogDescription>
						What the coding agent has written so far. Refreshes every 10
						seconds.
					</DialogDescription>
				</DialogHeader>
				{actionKey !== null && (
					<ContainerLogsBody
						key={actionKey}
						dialogId={dialogId}
						actionKey={actionKey}
					/>
				)}
			</DialogContent>
		</Dialog>
	)
}

interface ContainerLogsBodyProps {
	dialogId: string
	actionKey: string
}

function ContainerLogsBody({ dialogId, actionKey }: ContainerLogsBodyProps) {
	const [logs, setLogs] = useState('')
	const [truncated, setTruncated] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [loading, setLoading] = useState(true)
	const [refreshing, setRefreshing] = useState(false)

	const paneRef = useRef<HTMLDivElement>(null)
	// Kept out of state: it changes on every scroll event, and re-rendering the
	// pane while someone is reading it would fight the scroll.
	const stickRef = useRef(true)

	const handleScroll = useCallback(() => {
		const el = paneRef.current
		if (!el) return
		stickRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 32
	}, [])

	useLayoutEffect(() => {
		const el = paneRef.current
		if (el && stickRef.current) el.scrollTop = el.scrollHeight
	}, [logs])

	useEffect(() => {
		let cancelled = false
		let timer: ReturnType<typeof setTimeout> | undefined
		const controller = new AbortController()

		const tick = async () => {
			try {
				const res = await getActionContainerLogs(
					dialogId,
					actionKey,
					controller.signal,
				)
				if (cancelled) return
				setLogs(res.logs)
				setTruncated(res.truncated)
				setError(null)
			} catch (err) {
				if (cancelled || controller.signal.aborted) return
				setError(
					err instanceof Error ? err.message : 'Failed to load container logs',
				)
				// The finally below still runs, so the first-load spinner clears.
				if (err instanceof ApiError && TERMINAL_STATUSES.has(err.status)) return
			} finally {
				if (!cancelled) setLoading(false)
			}
			// Scheduled only once the previous read has resolved, so a slow Docker
			// daemon cannot pile requests up or let an older snapshot land on top
			// of a newer one.
			if (!cancelled) timer = setTimeout(() => void tick(), REFRESH_MS)
		}

		void tick()

		return () => {
			cancelled = true
			controller.abort()
			if (timer) clearTimeout(timer)
		}
	}, [dialogId, actionKey])

	// The manual refresh deliberately leaves the timer alone: restarting it from
	// a click is how overlapping reads creep back in, and one extra read costs
	// nothing.
	const handleRefresh = useCallback(async () => {
		setRefreshing(true)
		try {
			const res = await getActionContainerLogs(dialogId, actionKey)
			setLogs(res.logs)
			setTruncated(res.truncated)
			setError(null)
		} catch (err) {
			setError(
				err instanceof Error ? err.message : 'Failed to load container logs',
			)
		} finally {
			setRefreshing(false)
		}
	}, [dialogId, actionKey])

	return (
		<div className="flex min-h-0 flex-col gap-3">
			{error !== null && (
				<p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
					{error}
				</p>
			)}

			{loading ? (
				<div className="flex h-24 items-center justify-center gap-2 text-sm text-muted-foreground">
					<Loader2 className="h-4 w-4 animate-spin" />
					Loading container logs...
				</div>
			) : (
				// A failed refresh keeps the last good snapshot on screen; only a
				// first read that never succeeded leaves the error standing alone.
				(logs !== '' || error === null) && (
					<div
						ref={paneRef}
						onScroll={handleScroll}
						className="max-h-[60vh] min-h-[8rem] overflow-auto rounded-md border bg-muted/40 p-3"
					>
						<pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed">
							{logs === '' ? 'The container has written nothing yet.' : logs}
						</pre>
					</div>
				)
			)}

			<div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
				<span>
					{truncated
						? 'Showing the most recent output; earlier lines were dropped.'
						: 'Refreshes every 10 seconds.'}
				</span>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onClick={() => void handleRefresh()}
					disabled={refreshing}
				>
					<RefreshCw
						className={refreshing ? 'h-3.5 w-3.5 animate-spin' : 'h-3.5 w-3.5'}
					/>
					Refresh
				</Button>
			</div>
		</div>
	)
}
