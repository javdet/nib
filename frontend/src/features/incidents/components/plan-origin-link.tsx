import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router'
import { AlertTriangle } from 'lucide-react'
import { useMode } from '@/features/modes/mode-context'
import { useDialog } from '@/features/dialogs/dialog-context'
import { getPlanOrigin, type PlanOrigin } from '../api/incidents'

/**
 * Names the incident a plan was created from, on the plan's own page. Renders
 * nothing for a plan created any other way, or when the origin cannot be read:
 * the link is provenance, and the plan stands without it.
 */
export function PlanOriginLink({ planId }: { planId: string }) {
	const navigate = useNavigate()
	const { selectMode } = useMode()
	const { setActiveDialogId } = useDialog()
	const [origin, setOrigin] = useState<PlanOrigin | null>(null)

	useEffect(() => {
		let cancelled = false
		setOrigin(null)
		getPlanOrigin(planId)
			.then((value) => {
				if (!cancelled) setOrigin(value)
			})
			.catch(() => {})
		return () => {
			cancelled = true
		}
	}, [planId])

	if (!origin) return null

	const handleOpen = () => {
		selectMode('incident')
		setActiveDialogId(origin.incidentId)
		void navigate(`/incidents/${origin.incidentId}`)
	}

	return (
		<div className="flex justify-center">
			<button
				type="button"
				onClick={handleOpen}
				className="inline-flex items-center gap-1.5 rounded-full border border-border/60 bg-secondary/50 px-3 py-1 text-xs text-muted-foreground transition-colors hover:bg-secondary hover:text-foreground"
			>
				<AlertTriangle className="h-3.5 w-3.5" />
				From incident {origin.incidentTitle ? `«${origin.incidentTitle}»` : 'post-mortem'}
			</button>
		</div>
	)
}
