import { Select } from '@/components/ui/select'
import { RANGE_PRESETS, type RangePresetId } from '../lib/ranges'

interface RangeSelectProps {
	value: RangePresetId
	onChange: (value: RangePresetId) => void
	disabled?: boolean
}

/**
 * RangeSelect offers presets rather than a free date range.
 *
 * Each preset pairs its window with a bucket that keeps the request inside the
 * backend's bucket cap by construction, so the UI cannot offer a range the API
 * will reject.
 */
export function RangeSelect({ value, onChange, disabled }: RangeSelectProps) {
	return (
		<Select
			aria-label="Statistics range"
			value={value}
			disabled={disabled}
			onChange={(e) => onChange(e.target.value as RangePresetId)}
			wrapperClassName="w-48"
		>
			{RANGE_PRESETS.map((preset) => (
				<option key={preset.id} value={preset.id}>
					{preset.label}
				</option>
			))}
		</Select>
	)
}
