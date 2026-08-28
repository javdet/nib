import { api } from '@/lib/api-client'

export interface MCPServerEntry {
	url?: string
	transport?: string
	headers?: Record<string, string>
	command?: string
	args?: string[]
	env?: Record<string, string>
	description?: string
}

export interface MCPServer extends MCPServerEntry {
	name: string
}

interface MCPRawResponse {
	content: string
}

export function listMCPServers(): Promise<MCPServer[]> {
	return api.get<MCPServer[]>('/mcp/servers')
}

export function getMCPServer(name: string): Promise<MCPServer> {
	return api.get<MCPServer>(`/mcp/servers/${encodeURIComponent(name)}`)
}

export function createMCPServer(server: MCPServer): Promise<MCPServer> {
	return api.post<MCPServer>('/mcp/servers', server)
}

export function updateMCPServer(
	oldName: string,
	server: MCPServer,
): Promise<void> {
	return api.put<void>(
		`/mcp/servers/${encodeURIComponent(oldName)}`,
		server,
	)
}

export function deleteMCPServer(name: string): Promise<void> {
	return api.delete<void>(`/mcp/servers/${encodeURIComponent(name)}`)
}

export interface MCPTool {
	name: string
	description: string
	inputSchema?: Record<string, unknown>
}

export function listMCPServerTools(serverName: string): Promise<MCPTool[]> {
	return api.get<MCPTool[]>(
		`/mcp/servers/${encodeURIComponent(serverName)}/tools`,
	)
}

export function getMCPConfigRaw(): Promise<string> {
	return api
		.get<MCPRawResponse>('/mcp/config/raw')
		.then((res) => res.content)
}

export function updateMCPConfigRaw(content: string): Promise<void> {
	return api.put<void>('/mcp/config/raw', { content })
}
