import {
	CircleEllipsis,
	CircleHelp,
	FileCode2,
	Globe,
	Send,
	SquareTerminal,
	type LucideIcon,
} from 'lucide-react'

const actionTypeIcons: Record<string, LucideIcon> = {
	code: FileCode2,
	web: Globe,
	curl: Send,
	shell: SquareTerminal,
	other: CircleEllipsis,
}

export function actionTypeIcon(type: string): LucideIcon {
	const key = type.trim().toLowerCase()
	return actionTypeIcons[key] ?? CircleHelp
}
