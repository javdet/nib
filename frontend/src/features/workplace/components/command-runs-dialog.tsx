import { useCallback, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Loader2, ShieldAlert, Square } from 'lucide-react'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import {
	cancelCommandRun,
	getCommandRunOutput,
	listCommandRuns,
	type CommandRun,
} from '@/features/dialogs/api/dialogs'
import { ApiError } from '@/lib/api-client'
import { cn } from '@/lib/utils'
import {
	appendOutput,
	commandRunLabel,
	formatDuration,
	scriptSummary,
} from '../lib/command-runs'

const LIST_REFRESH_MS = 5_000
const OUTPUT_REFRESH_MS = 3_000
// Past this much text the pane stops reading on its own: a command may keep
// 10 MiB, and a <pre> that size makes the page crawl. The rest is a click away.
const AUTO_LOAD_LIMIT = 1 << 20

interface CommandRunsDialogProps {
	dialogId: string
	/** The action row whose commands are shown, or null when closed. */
	actionKey: string | null
	number?: string
	onClose: () => void
}

/**
 * The commands an action's sub-agent ran, each with its output.
 *
 * Like the container logs, the body is a child of DialogContent so closing the
 * dialog unmounts it, and the polling effects' cleanups are what stop them.
 */
export function CommandRunsDialog({
	dialogId,
	actionKey,
	number,
	onClose,
}: CommandRunsDialogProps) {
	return (
		<Dialog
			open={actionKey !== null}
			onOpenChange={(open) => {
				if (!open) onClose()
			}}
		>
			<DialogContent className="max-w-4xl">
				<DialogHeader>
					<DialogTitle>Commands{number ? ` — ${number}` : ''}</DialogTitle>
					<DialogDescription>
						What the agent ran for this action, newest first. A running
						command's output refreshes every few seconds.
					</DialogDescription>
				</DialogHeader>
				{actionKey !== null && (
					<CommandRunsBody key={actionKey} dialogId={dialogId} actionKey={actionKey} />
				)}
			</DialogContent>
		</Dialog>
	)
}

function CommandRunsBody({
	dialogId,
	actionKey,
}: {
	dialogId: string
	actionKey: string
}) {
	const [runs, setRuns] = useState<CommandRun[] | null>(null)
	const [error, setError] = useState<string | null>(null)
	const [selected, setSelected] = useState<string | null>(null)
	const [reload, setReload] = useState(0)

	useEffect(() => {
		let cancelled = false
		let timer: ReturnType<typeof setTimeout> | undefined
		const controller = new AbortController()

		// Polled for as long as the dialog is open: the agent may start another
		// command at any moment, and one list read is cheap.
		const tick = async () => {
			try {
				const res = await listCommandRuns(dialogId, actionKey, controller.signal)
				if (cancelled) return
				setRuns(res.runs)
				setError(null)
			} catch (err) {
				if (cancelled || controller.signal.aborted) return
				setError(err instanceof Error ? err.message : 'Failed to load commands')
				if (err instanceof ApiError && err.status === 503) return
			}
			if (!cancelled) timer = setTimeout(() => void tick(), LIST_REFRESH_MS)
		}
		void tick()

		return () => {
			cancelled = true
			controller.abort()
			if (timer) clearTimeout(timer)
		}
	}, [dialogId, actionKey, reload])

	const current = runs?.find((r) => r.id === selected) ?? runs?.[0] ?? null

	return (
		<div className="flex min-h-0 min-w-0 flex-col gap-3">
			{error !== null && (
				<p className="rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
					{error}
				</p>
			)}

			{runs === null ? (
				error === null && (
					<div className="flex h-24 items-center justify-center gap-2 text-sm text-muted-foreground">
						<Loader2 className="h-4 w-4 animate-spin" />
						Loading commands...
					</div>
				)
			) : runs.length === 0 ? (
				<p className="py-6 text-center text-sm text-muted-foreground">
					The agent has run no commands for this action.
				</p>
			) : (
				<>
					<ul className="max-h-48 overflow-auto rounded-md border">
						{runs.map((run) => (
							<li key={run.id}>
								<button
									type="button"
									onClick={() => setSelected(run.id)}
									className={cn(
										'flex w-full min-w-0 items-center gap-3 px-3 py-1.5 text-left text-xs hover:bg-muted/60 focus-ring',
										current?.id === run.id && 'bg-muted',
									)}
								>
									<StatusDot run={run} />
									<code className="min-w-0 flex-1 truncate font-mono">
										{run.kind === 'wait_until' ? 'wait_until: ' : ''}
										{scriptSummary(run.script)}
									</code>
									<span className="shrink-0 text-muted-foreground">
										{commandRunLabel(run)}
										{run.status !== 'running' ? ` · ${formatDuration(run.duration_ms)}` : ''}
									</span>
								</button>
							</li>
						))}
					</ul>
					{current && (
						<CommandRunDetail
							key={current.id}
							dialogId={dialogId}
							run={current}
							onCancelled={() => setReload((n) => n + 1)}
						/>
					)}
				</>
			)}
		</div>
	)
}

function StatusDot({ run }: { run: CommandRun }) {
	return (
		<span
			aria-hidden
			className={cn(
				'h-2 w-2 shrink-0 rounded-full',
				run.status === 'running' && 'animate-pulse bg-sky-500',
				run.status === 'succeeded' && 'bg-emerald-500',
				(run.status === 'failed' || run.status === 'timed_out') && 'bg-destructive',
				(run.status === 'cancelled' || run.status === 'lost') && 'bg-muted-foreground',
			)}
		/>
	)
}

function CommandRunDetail({
	dialogId,
	run,
	onCancelled,
}: {
	dialogId: string
	run: CommandRun
	onCancelled: () => void
}) {
	const [cancelling, setCancelling] = useState(false)
	const [cancelError, setCancelError] = useState<string | null>(null)

	const handleCancel = useCallback(async () => {
		setCancelling(true)
		setCancelError(null)
		try {
			await cancelCommandRun(dialogId, run.id)
			onCancelled()
		} catch (err) {
			setCancelError(err instanceof Error ? err.message : 'Failed to cancel')
		} finally {
			setCancelling(false)
		}
	}, [dialogId, run.id, onCancelled])

	return (
		<div className="flex min-w-0 flex-col gap-2">
			<pre className="max-h-32 overflow-auto rounded-md border bg-background/70 p-2 font-mono text-xs whitespace-pre-wrap [overflow-wrap:anywhere]">
				{run.script}
			</pre>
			<div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
				<span>{commandRunLabel(run)}</span>
				{run.reason ? <span>— {run.reason}</span> : null}
				{run.isolated ? (
					<span>sandbox uid {run.uid}</span>
				) : (
					<span className="flex items-center gap-1 text-amber-600 dark:text-amber-400">
						<ShieldAlert className="h-3.5 w-3.5" aria-hidden />
						ran as the backend's own user
					</span>
				)}
				<span className="flex-1" />
				{run.status === 'running' && (
					<Button
						type="button"
						variant="outline"
						size="sm"
						onClick={() => void handleCancel()}
						disabled={cancelling}
					>
						<Square className="h-3.5 w-3.5" />
						Cancel
					</Button>
				)}
			</div>
			{cancelError !== null && (
				<p className="text-xs text-destructive">{cancelError}</p>
			)}
			<CommandOutputPane dialogId={dialogId} runId={run.id} />
		</div>
	)
}

function CommandOutputPane({ dialogId, runId }: { dialogId: string; runId: string }) {
	const [shown, setShown] = useState({ text: '', next: 0 })
	const [eof, setEof] = useState(false)
	const [capped, setCapped] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const [loading, setLoading] = useState(true)
	const [more, setMore] = useState(0)

	const nextRef = useRef(0)
	const paneRef = useRef<HTMLDivElement>(null)
	const stickRef = useRef(true)

	const handleScroll = useCallback(() => {
		const el = paneRef.current
		if (!el) return
		stickRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 32
	}, [])

	useLayoutEffect(() => {
		const el = paneRef.current
		if (el && stickRef.current) el.scrollTop = el.scrollHeight
	}, [shown])

	useEffect(() => {
		let cancelled = false
		let timer: ReturnType<typeof setTimeout> | undefined
		const controller = new AbortController()
		// Each effect run (the first, or a "load more" click) may read this much
		// before it stops for the operator to ask for more.
		const budget = nextRef.current + AUTO_LOAD_LIMIT

		const tick = async () => {
			let delay: number | null = null
			try {
				const page = await getCommandRunOutput(
					dialogId,
					runId,
					nextRef.current,
					controller.signal,
				)
				if (cancelled) return
				if (page.offset === nextRef.current) nextRef.current = page.next_offset
				setShown((prev) => appendOutput(prev, page))
				setEof(page.eof)
				setCapped(page.capped)
				setError(null)
				if (!page.eof && nextRef.current < budget) delay = 0
				else if (page.status === 'running' && nextRef.current < budget) delay = OUTPUT_REFRESH_MS
			} catch (err) {
				if (cancelled || controller.signal.aborted) return
				setError(err instanceof Error ? err.message : 'Failed to load output')
				// 404 and 410 will not change; anything else is worth a retry.
				if (!(err instanceof ApiError && (err.status === 404 || err.status === 410))) {
					delay = OUTPUT_REFRESH_MS
				}
			} finally {
				if (!cancelled) setLoading(false)
			}
			if (!cancelled && delay !== null) timer = setTimeout(() => void tick(), delay)
		}
		void tick()

		return () => {
			cancelled = true
			controller.abort()
			if (timer) clearTimeout(timer)
		}
	}, [dialogId, runId, more])

	const stopped = !eof && shown.next >= AUTO_LOAD_LIMIT * (more + 1)

	return (
		<div className="flex min-w-0 flex-col gap-2">
			{error !== null && <p className="text-xs text-destructive">{error}</p>}
			{loading ? (
				<div className="flex h-24 items-center justify-center gap-2 text-sm text-muted-foreground">
					<Loader2 className="h-4 w-4 animate-spin" />
					Loading output...
				</div>
			) : (
				<div
					ref={paneRef}
					onScroll={handleScroll}
					className="max-h-[50vh] min-h-[8rem] overflow-auto rounded-md border bg-muted/40 p-3"
				>
					<pre className="whitespace-pre-wrap [overflow-wrap:anywhere] font-mono text-xs leading-relaxed">
						{shown.text === '' ? 'No output yet.' : shown.text}
					</pre>
				</div>
			)}
			<div className="flex items-center justify-between gap-2 text-xs text-muted-foreground">
				<span>
					{capped
						? 'Output past 10 MiB was not kept.'
						: `${shown.next.toLocaleString()} bytes`}
				</span>
				{stopped && (
					<Button type="button" variant="outline" size="sm" onClick={() => setMore((n) => n + 1)}>
						Load more
					</Button>
				)}
			</div>
		</div>
	)
}
