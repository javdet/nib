import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import type { UsageKey } from '../api/statistics'
import { formatCost, formatTokens } from '../lib/format'

interface BreakdownTableProps {
	title: string
	label: string
	rows: UsageKey[]
	emptyMessage?: string
}

export function BreakdownTable({
	title,
	label,
	rows,
	emptyMessage = 'Nothing recorded in this range yet.',
}: BreakdownTableProps) {
	return (
		<Card>
			<CardHeader className="pb-2">
				<CardTitle className="text-base">{title}</CardTitle>
			</CardHeader>
			<CardContent>
				{rows.length === 0 ? (
					<p className="py-6 text-center text-sm text-muted-foreground">{emptyMessage}</p>
				) : (
					<div className="overflow-x-auto">
						<Table>
							<TableHeader>
								<TableRow>
									<TableHead>{label}</TableHead>
									<TableHead className="text-right">Calls</TableHead>
									<TableHead className="text-right">Tokens</TableHead>
									<TableHead className="text-right">Cost</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{rows.map((row) => (
									<TableRow key={row.key}>
										<TableCell className="font-medium">{row.key || '—'}</TableCell>
										<TableCell className="text-right">
											{row.calls.toLocaleString('en-US')}
										</TableCell>
										<TableCell className="text-right">
											{formatTokens(row.totalTokens)}
										</TableCell>
										<TableCell className="text-right">{formatCost(row.costUsd)}</TableCell>
									</TableRow>
								))}
							</TableBody>
						</Table>
					</div>
				)}
			</CardContent>
		</Card>
	)
}
