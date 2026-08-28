import { useState, useEffect, useCallback } from 'react'
import {
	Dialog,
	DialogContent,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import { extractErrorMessage } from '@/lib/api-client'
import { MCPToolsList } from '@/features/mcp-connections/components/mcp-tools-list'
import {
	listMCPServerTools,
	type MCPServer,
	type MCPTool,
} from '../api/mcp-config'

interface MCPServerToolsDialogProps {
	open: boolean
	onOpenChange: (open: boolean) => void
	server: MCPServer | null
}

export function MCPServerToolsDialog({
	open,
	onOpenChange,
	server,
}: MCPServerToolsDialogProps) {
	const [tools, setTools] = useState<MCPTool[]>([])
	const [loading, setLoading] = useState(false)
	const [error, setError] = useState<string | null>(null)

	const fetchTools = useCallback(async () => {
		if (!server?.name) return
		setLoading(true)
		setError(null)
		try {
			const result = await listMCPServerTools(server.name)
			setTools(result)
		} catch (err) {
			setError(extractErrorMessage(err))
			setTools([])
		} finally {
			setLoading(false)
		}
	}, [server?.name])

	useEffect(() => {
		if (open && server?.name) {
			void fetchTools()
		} else if (!open) {
			setTools([])
			setError(null)
			setLoading(false)
		}
	}, [open, server?.name, fetchTools])

	return (
		<Dialog open={open} onOpenChange={onOpenChange}>
			<DialogContent className="sm:max-w-2xl max-h-[85vh] overflow-y-auto">
				<DialogHeader>
					<DialogTitle>
						Tools — {server?.name ?? ''}
					</DialogTitle>
				</DialogHeader>
				<MCPToolsList
					tools={tools}
					loading={loading}
					error={error}
					onRefresh={() => void fetchTools()}
				/>
			</DialogContent>
		</Dialog>
	)
}
