import { useState } from 'react'
import { CheckCircle2, Circle, Loader2, Square, XCircle } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { extractErrorMessage } from '@/lib/api-client'
import { cn } from '@/lib/utils'
import type { IncidentRun, IncidentStepStatus } from '../api/incidents'
import { investigationProgress, stepLabel } from '../lib/investigation-progress'

const STATUS_ICONS: Record<IncidentStepStatus, { icon: typeof Circle; className: string }> = {
	pending: { icon: Circle, className: 'text-muted-foreground' },
	running: { icon: Loader2, className: 'animate-spin text-blue-500' },
	done: { icon: CheckCircle2, className: 'text-lime-600 dark:text-lime-400' },
	failed: { icon: XCircle, className: 'text-destructive' },
}

interface InvestigationCardProps {
	run: IncidentRun
	onStop: () => Promise<void>
	onOpenTranscript: (dialogId: string) => void
}

/** The latest investigation or closure run: one row per agent. */
export function InvestigationCard({ run, onStop, onOpenTranscript }: InvestigationCardProps) {
	const [stopping, setStopping] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const progress = investigationProgress(run)

	const handleStop = async () => {
		setStopping(true)
		setError(null)
		try {
			await onStop()
		} catch (err) {
			setError(extractErrorMessage(err))
		} finally {
			setStopping(false)
		}
	}

	return (
		<Card>
			<CardHeader>
				<div className="flex items-center gap-2">
					<CardTitle className="text-base">{progress.headline}</CardTitle>
					{progress.running && (
						<Button
							type="button"
							variant="outline"
							size="sm"
							className="ml-auto"
							disabled={stopping}
							onClick={() => void handleStop()}
						>
							<Square className="mr-2 h-3.5 w-3.5" />
							{stopping ? 'Stopping...' : 'Stop'}
						</Button>
					)}
				</div>
				{progress.running && (
					<div className="mt-2 h-1.5 w-full overflow-hidden rounded-full bg-muted">
						<div
							className="h-full bg-primary transition-[width] duration-500"
							style={{ width: `${Math.round(((progress.done + progress.failed) / Math.max(progress.total, 1)) * 100)}%` }}
						/>
					</div>
				)}
			</CardHeader>
			<CardContent className="space-y-1">
				{run.steps.map((step, i) => {
					const { icon: Icon, className } = STATUS_ICONS[step.status] ?? STATUS_ICONS.pending
					return (
						<div key={`${step.kind}-${step.system ?? ''}-${i}`} className="flex items-start gap-2 text-sm">
							<Icon className={cn('mt-0.5 h-4 w-4 shrink-0', className)} />
							<div className="min-w-0 flex-1">
								<span className={cn(step.kind !== 'digger' && 'text-muted-foreground')}>
									{stepLabel(step)}
								</span>
								{step.focus && (
									<span className="text-muted-foreground"> — {step.focus}</span>
								)}
								{step.error && (
									<p className="text-xs text-destructive">{step.error}</p>
								)}
							</div>
							{step.dialogId && (
								<button
									type="button"
									className="shrink-0 text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
									onClick={() => onOpenTranscript(step.dialogId!)}
								>
									Transcript
								</button>
							)}
						</div>
					)
				})}
				{error && <p className="pt-2 text-sm text-destructive">{error}</p>}
			</CardContent>
		</Card>
	)
}
