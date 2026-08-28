import { Bot } from 'lucide-react'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import type { TableData } from '@/features/dialogs/api/dialogs'

interface TableMessageProps {
	table: TableData
}

export function TableMessage({ table }: TableMessageProps) {
	return (
		<div className="flex min-w-0 flex-row gap-2">
			<div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full border bg-muted">
				<Bot className="h-3.5 w-3.5" />
			</div>
			<div className="min-w-0 max-w-[85%] rounded-lg bg-muted px-3 py-2 text-sm">
				{table.title && (
					<p className="mb-2 font-medium">{table.title}</p>
				)}
				<div className="overflow-x-auto">
					<Table>
						<TableHeader>
							<TableRow className="hover:bg-transparent">
								{table.columns.map((col) => (
									<TableHead
										key={col}
										className="h-8 whitespace-nowrap px-2 text-xs font-medium"
									>
										{col}
									</TableHead>
								))}
							</TableRow>
						</TableHeader>
						<TableBody>
							{table.rows.map((row, rowIndex) => (
								<TableRow
									key={rowIndex}
									className="hover:bg-muted/80"
								>
									{row.map((cell, cellIndex) => (
										<TableCell
											key={cellIndex}
											className="whitespace-pre-line px-2 py-1.5 align-top text-xs"
										>
											{cell || '\u00a0'}
										</TableCell>
									))}
								</TableRow>
							))}
						</TableBody>
					</Table>
				</div>
			</div>
		</div>
	)
}
