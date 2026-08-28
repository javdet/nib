import { KeyValueDialog } from '@/features/variables/components/key-value-dialog'
import type { KeyValueInput } from '@/features/variables/components/key-value-dialog'
import {
	linesToListValue,
	listValueToLines,
	type Variable,
	type VariableInput,
} from '../api/variables'

interface VariableDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	onSave: (input: VariableInput) => void
	initialVariable?: Variable | null
}

function toDialogInput(variable: Variable): KeyValueInput {
	const kind = variable.kind ?? 'string'
	return {
		scope: variable.scope,
		name: variable.name,
		description: variable.description,
		value: kind === 'list' ? listValueToLines(variable.value) : variable.value,
		kind,
	}
}

function toVariableInput(input: KeyValueInput): VariableInput {
	const kind = input.kind ?? 'string'
	return {
		scope: input.scope,
		name: input.name,
		description: input.description,
		kind,
		value:
			kind === 'list' ? linesToListValue(input.value) : input.value,
	}
}

export function VariableDialog({
	open,
	onOpenChange,
	onSave,
	initialVariable,
}: VariableDialogProps) {
	return (
		<KeyValueDialog
			open={open}
			onOpenChange={onOpenChange}
			kind="variable"
			onSave={(input) => onSave(toVariableInput(input))}
			initial={
				initialVariable ? toDialogInput(initialVariable) : null
			}
		/>
	)
}
