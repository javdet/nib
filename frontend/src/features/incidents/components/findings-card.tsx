import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { MarkdownMessage } from '@/components/markdown-message'
import { cn } from '@/lib/utils'
import type { PostmortemFinding } from '../api/incidents'
import { findingStatusStyle } from '../lib/postmortem-labels'

interface FindingsCardProps {
	findings: PostmortemFinding[]
	onOpenTranscript: (dialogId: string) => void
}

function Detail({ label, value }: { label: string; value?: string }) {
	if (!value) return null
	return (
		<p>
			<span className="font-medium text-muted-foreground">{label}: </span>
			<span className="whitespace-pre-wrap">{value}</span>
		</p>
	)
}

/** What each Digger established about its system. Written only by the Diggers. */
export function FindingsCard({ findings, onOpenTranscript }: FindingsCardProps) {
	if (findings.length === 0) return null

	return (
		<Card>
			<CardHeader>
				<CardTitle>Findings by system</CardTitle>
			</CardHeader>
			<CardContent className="space-y-4">
				{findings.map((finding) => {
					const style = findingStatusStyle(finding.status)
					return (
						<div key={finding.system} className="space-y-2 rounded-md border border-border/60 p-3">
							<div className="flex flex-wrap items-center gap-2">
								<span className="font-medium">{finding.system}</span>
								{finding.environment && (
									<span className="text-sm text-muted-foreground">{finding.environment}</span>
								)}
								<span
									className={cn(
										'rounded-full border px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wider',
										style.className,
									)}
								>
									{style.label}
								</span>
								{finding.confidence && (
									<span className="text-xs text-muted-foreground">
										confidence: {finding.confidence}
									</span>
								)}
								{finding.dialogId && (
									<button
										type="button"
										className="ml-auto text-xs text-muted-foreground underline-offset-2 hover:text-foreground hover:underline"
										onClick={() => onOpenTranscript(finding.dialogId!)}
									>
										Transcript
									</button>
								)}
							</div>
							<div className="text-sm">
								<MarkdownMessage content={finding.summary} onDialogLink={onOpenTranscript} />
							</div>
							<div className="space-y-1 text-sm">
								<Detail label="Before" value={finding.before} />
								<Detail label="During" value={finding.during} />
								<Detail label="Suspected cause" value={finding.suspectedCause} />
								<Detail label="Assumptions" value={finding.assumptions} />
								{finding.changes && finding.changes.length > 0 && (
									<div>
										<span className="font-medium text-muted-foreground">Changes:</span>
										<ul className="ml-5 list-disc">
											{finding.changes.map((change) => (
												<li key={change}>{change}</li>
											))}
										</ul>
									</div>
								)}
							</div>
						</div>
					)
				})}
			</CardContent>
		</Card>
	)
}
