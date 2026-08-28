import { useEffect, useMemo, useState } from 'react'
import { format } from 'date-fns'
import { CalendarIcon } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from '@/components/ui/popover'
import { cn } from '@/lib/utils'

interface PlanScheduleFieldProps {
	scheduledAt: number
	onChange: (nextUnix: number) => void
	disabled?: boolean
	hideLabel?: boolean
}

const DEFAULT_TIME = '09:00'

function unixToDate(unix: number): Date | undefined {
	if (unix <= 0) return undefined
	return new Date(unix * 1000)
}

function formatTimeFromDate(date: Date): string {
	const hours = String(date.getHours()).padStart(2, '0')
	const minutes = String(date.getMinutes()).padStart(2, '0')
	return `${hours}:${minutes}`
}

function dateTimeToUnix(date: Date, time: string): number {
	const [hours, minutes] = time.split(':').map(Number)
	const next = new Date(date)
	next.setHours(hours ?? 0, minutes ?? 0, 0, 0)
	return Math.floor(next.getTime() / 1000)
}

export function PlanScheduleField({
	scheduledAt,
	onChange,
	disabled = false,
	hideLabel = false,
}: PlanScheduleFieldProps) {
	const [open, setOpen] = useState(false)
	const selectedDate = useMemo(() => unixToDate(scheduledAt), [scheduledAt])
	const [draftDate, setDraftDate] = useState<Date | undefined>(selectedDate)
	const [draftTime, setDraftTime] = useState(
		scheduledAt > 0 ? formatTimeFromDate(new Date(scheduledAt * 1000)) : DEFAULT_TIME,
	)

	useEffect(() => {
		setDraftDate(unixToDate(scheduledAt))
		if (scheduledAt > 0) {
			setDraftTime(formatTimeFromDate(new Date(scheduledAt * 1000)))
		}
	}, [scheduledAt])

	const handleDateSelect = (date: Date | undefined) => {
		setDraftDate(date)
		if (!date) return
		onChange(dateTimeToUnix(date, draftTime))
	}

	const handleTimeChange = (time: string) => {
		setDraftTime(time)
		if (draftDate) {
			onChange(dateTimeToUnix(draftDate, time))
		}
	}

	const handleClear = () => {
		onChange(0)
		setDraftDate(undefined)
		setDraftTime(DEFAULT_TIME)
		setOpen(false)
	}

	const label =
		scheduledAt > 0
			? format(new Date(scheduledAt * 1000), 'PPP p')
			: 'Select schedule'

	return (
		<div className={cn('flex items-center', !hideLabel && 'gap-3')}>
			{!hideLabel && (
				<span className="text-sm font-medium">Date:</span>
			)}
			<Popover open={open} onOpenChange={setOpen}>
				<PopoverTrigger asChild>
					<Button
						type="button"
						variant="outline"
						disabled={disabled}
						className={cn(
							'w-[280px] justify-start text-left font-normal',
							scheduledAt <= 0 && 'text-muted-foreground',
						)}
					>
						<CalendarIcon className="mr-2 h-4 w-4" />
						{label}
					</Button>
				</PopoverTrigger>
				<PopoverContent className="w-auto p-0" align="start">
					<Calendar
						mode="single"
						selected={draftDate}
						onSelect={handleDateSelect}
						disabled={disabled}
						autoFocus
					/>
					<div className="space-y-2 border-t p-3">
						<Label htmlFor="schedule-time">Time</Label>
						<Input
							id="schedule-time"
							type="time"
							value={draftTime}
							onChange={(event) => handleTimeChange(event.target.value)}
							disabled={disabled || !draftDate}
						/>
						<Button
							type="button"
							variant="ghost"
							size="sm"
							className="w-full"
							onClick={handleClear}
							disabled={disabled || scheduledAt <= 0}
						>
							Clear schedule
						</Button>
					</div>
				</PopoverContent>
			</Popover>
		</div>
	)
}
