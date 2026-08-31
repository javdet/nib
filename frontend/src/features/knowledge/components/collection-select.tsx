import { useCallback, useRef, useState } from 'react'
import { Check, ChevronDown, Plus, X } from 'lucide-react'
import { Input } from '@/components/ui/input'
import { IconButton } from '@/components/ui/icon-button'
import {
	DropdownMenu,
	DropdownMenuContent,
	DropdownMenuItem,
	DropdownMenuSeparator,
	DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import {
	type KnowledgeCollection,
	isValidKnowledgeCollectionName,
} from '../api/knowledge'

interface CollectionSelectProps {
	collections: KnowledgeCollection[]
	value: string
	onChange: (name: string) => void
	disabled?: boolean
}

export function CollectionSelect({
	collections,
	value,
	onChange,
	disabled = false,
}: CollectionSelectProps) {
	const [mode, setMode] = useState<'select' | 'create'>('select')
	const [previousValue, setPreviousValue] = useState(value)
	const inputRef = useRef<HTMLInputElement>(null)

	const trimmedValue = value.trim()
	const isValid = isValidKnowledgeCollectionName(value)
	const displayName = trimmedValue || 'default'

	const handleSelect = useCallback(
		(name: string) => {
			onChange(name)
			setMode('select')
		},
		[onChange],
	)

	const handleStartCreate = useCallback(() => {
		setPreviousValue(value)
		onChange('')
		setMode('create')
	}, [onChange, value])

	const handleCancelCreate = useCallback(() => {
		onChange(previousValue)
		setMode('select')
	}, [onChange, previousValue])

	const handleCommitCreate = useCallback(() => {
		if (!isValidKnowledgeCollectionName(value)) return
		setMode('select')
	}, [value])

	const handleCreateKeyDown = useCallback(
		(e: React.KeyboardEvent<HTMLInputElement>) => {
			if (e.key === 'Enter') {
				e.preventDefault()
				handleCommitCreate()
			} else if (e.key === 'Escape') {
				e.preventDefault()
				handleCancelCreate()
			}
		},
		[handleCommitCreate, handleCancelCreate],
	)

	if (mode === 'create') {
		return (
			<div className="space-y-2">
				<div className="flex items-center gap-2">
					<Input
						ref={inputRef}
						id="kb-collection"
						value={value}
						onChange={(e) => onChange(e.target.value)}
						onKeyDown={handleCreateKeyDown}
						onBlur={handleCommitCreate}
						placeholder="my-collection"
						disabled={disabled}
						autoComplete="off"
						autoFocus
					/>
					<IconButton
						type="button"
						variant="outline"
						size="icon"
						className="shrink-0"
						onClick={handleCancelCreate}
						disabled={disabled}
						tooltip="Cancel"
					>
						<X className="h-4 w-4" />
					</IconButton>
				</div>
				{trimmedValue.length > 0 && !isValid && (
					<p className="text-xs text-destructive">
						Use 1–64 characters: letters, numbers, dots, underscores, or
						hyphens. Must start with a letter or number.
					</p>
				)}
			</div>
		)
	}

	return (
		<DropdownMenu>
			<DropdownMenuTrigger asChild disabled={disabled}>
				<button
					type="button"
					id="kb-collection"
					className={cn(
						'flex h-[var(--control-h)] w-full cursor-pointer items-center',
						'justify-between rounded-md border border-input bg-background/40',
						'px-3 py-1 text-base elev-1 focus-ring',
						'transition-[background-color,border-color,box-shadow]',
						'duration-[var(--dur-fast)] hover:border-ring/40',
						'disabled:cursor-not-allowed disabled:opacity-50 md:text-sm',
					)}
				>
					<span className="truncate">{displayName}</span>
					<ChevronDown className="ml-2 h-4 w-4 shrink-0 text-muted-foreground" />
				</button>
			</DropdownMenuTrigger>
			<DropdownMenuContent
				align="start"
				className="w-[var(--radix-dropdown-menu-trigger-width)] p-0"
			>
				<div className="max-h-72 overflow-y-auto p-1">
					{collections.length === 0 && (
						<p className="px-2 py-3 text-sm text-muted-foreground">
							No collections yet.
						</p>
					)}
					{collections.map((item) => {
						const isActive = item.name === trimmedValue
						return (
							<DropdownMenuItem
								key={item.name}
								onSelect={() => handleSelect(item.name)}
								className="flex items-center justify-between"
							>
								<span className="truncate">{item.name}</span>
								<span className="ml-2 flex shrink-0 items-center gap-2">
									<span className="text-xs text-muted-foreground">
										{item.chunkCount} chunks
									</span>
									{isActive && <Check className="h-4 w-4" />}
								</span>
							</DropdownMenuItem>
						)
					})}
				</div>
				<DropdownMenuSeparator />
				<div className="p-1">
					<DropdownMenuItem
						onSelect={handleStartCreate}
						className="gap-2"
					>
						<Plus className="h-4 w-4" />
						New collection
					</DropdownMenuItem>
				</div>
			</DropdownMenuContent>
		</DropdownMenu>
	)
}
