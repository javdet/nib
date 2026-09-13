import { Link } from 'react-router'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
import type { PlanUsage } from '../api/statistics'
import { formatCost, formatTokens } from '../lib/format'

interface TopPlansTableProps {
	rows: PlanUsage[]
}

export function TopPlansTable({ rows }: TopPlansTableProps) {
	return (
		<Card>
			<CardHeader className="pb-2">
				<CardTitle className="text-base">Most expensive tasks</CardTitle>
			</CardHeader>
			<CardContent>
				{rows.length === 0 ? (
					<p className="py-6 text-center text-sm text-muted-foreground">
						No task-attributed usage in this range yet.
					</p>
				) : (
					<div className="overflow-x-auto">
						{/* table-fixed with explicit widths: task titles are long and
						    free-form, and an auto layout lets one of them push the cost
						    column off the card instead of truncating itself. */}
						<Table className="table-fixed">
							<TableHeader>
								<TableRow>
									<TableHead className="w-[46%]">Task</TableHead>
									<TableHead className="w-[15%] text-right">Calls</TableHead>
									<TableHead className="w-[19%] text-right">Tokens</TableHead>
									<TableHead className="w-[20%] text-right">Cost</TableHead>
								</TableRow>
							</TableHeader>
							<TableBody>
								{rows.map((row) => (
									<TableRow key={row.planId}>
										<TableCell className="font-medium">
											{/* The clamp lives on a block child, not the cell: a <td> sizes
											    to its content, so max-w + truncate on the cell itself lets a
											    long title push the cost column out of the table. */}
											<div className="truncate">
												{/* Usage outlives the dialog it came from, so a deleted plan
												    keeps its spend and is shown by id. */}
												{row.title ? (
													<Link
														to={`/workplace/${row.planId}`}
														className="hover:underline focus-ring rounded-sm"
														title={row.title}
													>
														{row.title}
													</Link>
												) : (
													<span className="text-muted-foreground">
														Deleted task ({row.planId.slice(0, 8)})
													</span>
												)}
											</div>
										</TableCell>
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
