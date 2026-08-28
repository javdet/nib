import { TagInput } from '@/components/ui/tag-input'
import {
	Table,
	TableBody,
	TableCell,
	TableRow,
} from '@/components/ui/table'
import type { PlanStatus } from '@/features/dialogs/api/dialogs'
import { PlanScheduleField } from './plan-schedule-field'
import { PlanStatusBadge } from './plan-status-badge'

const LABEL_CLASS =
	'w-28 shrink-0 py-2 text-sm font-medium text-muted-foreground'

interface PlanMetaTableProps {
	planStatus: PlanStatus
	scheduledAt: number
	onScheduleChange: (nextUnix: number) => void
	scheduleDisabled?: boolean
	showSchedulingHint?: boolean
	components: string[]
	rules: string[]
	onComponentsChange: (next: string[]) => void
	onRulesChange: (next: string[]) => void
	disabled?: boolean
}

export function PlanMetaTable({
	planStatus,
	scheduledAt,
	onScheduleChange,
	scheduleDisabled = false,
	showSchedulingHint = false,
	components,
	rules,
	onComponentsChange,
	onRulesChange,
	disabled = false,
}: PlanMetaTableProps) {
	return (
		<Table>
			<TableBody className="[&_tr]:border-0">
				<TableRow className="hover:bg-transparent">
					<TableCell className={`${LABEL_CLASS} align-middle`}>
						Status
					</TableCell>
					<TableCell className="py-2 align-middle">
						<PlanStatusBadge status={planStatus} compact />
					</TableCell>
				</TableRow>
				<TableRow className="hover:bg-transparent">
					<TableCell className={`${LABEL_CLASS} align-top`}>Date</TableCell>
					<TableCell className="space-y-2 py-2">
						<PlanScheduleField
							scheduledAt={scheduledAt}
							onChange={onScheduleChange}
							disabled={scheduleDisabled}
							hideLabel
						/>
						{showSchedulingHint && (
							<p className="text-sm text-muted-foreground">
								Scheduling becomes available after decomposition produces a
								summary and DAG.
							</p>
						)}
					</TableCell>
				</TableRow>
				<TableRow className="hover:bg-transparent">
					<TableCell className={LABEL_CLASS}>Components</TableCell>
					<TableCell className="py-2">
						<TagInput
							value={components}
							onChange={onComponentsChange}
							variant="secondary"
							placeholder="Type a component and press Enter"
							disabled={disabled}
							aria-label="Components"
						/>
					</TableCell>
				</TableRow>
				<TableRow className="hover:bg-transparent">
					<TableCell className={LABEL_CLASS}>Rules</TableCell>
					<TableCell className="py-2">
						<TagInput
							value={rules}
							onChange={onRulesChange}
							variant="outline"
							placeholder="Type a rule and press Enter"
							disabled={disabled}
							aria-label="Rules"
						/>
					</TableCell>
				</TableRow>
			</TableBody>
		</Table>
	)
}
