import { describe, expect, it } from 'vitest'
import { stripJSONComments, validateMCPJSON } from './validate-mcp-json'

describe('stripJSONComments', () => {
	it('leaves comment-like sequences inside strings alone', () => {
		const text = '{"url": "https://gw.example.com/mcp"}'
		expect(stripJSONComments(text)).toBe(text)
	})

	it('removes line and block comments', () => {
		expect(
			stripJSONComments('{\n  // note\n  "a": 1, /* b */ "c": 2\n}'),
		).toBe('{\n  \n  "a": 1,  "c": 2\n}')
	})
})

describe('validateMCPJSON', () => {
	it('accepts a document with comments', () => {
		expect(
			validateMCPJSON(`{
				// the gateway
				"mcpServers": {"gw": {"url": "https://gw.example.com/mcp"}}
			}`),
		).toBeNull()
	})

	it('accepts a ${SECRET} reference that has no value yet', () => {
		expect(
			validateMCPJSON(
				'{"mcpServers": {"gw": {"headers": {"Authorization": "Bearer ${MCP_GW_TOKEN}"}}}}',
			),
		).toBeNull()
	})

	it('accepts keys the UI does not model', () => {
		expect(
			validateMCPJSON(
				'{"mcpServers": {"gw": {"type": "http", "url": "https://gw.example.com/mcp"}}, "inputs": []}',
			),
		).toBeNull()
	})

	it('reports why the document cannot be read back', () => {
		expect(validateMCPJSON('   ')).toBe('JSON cannot be empty')
		expect(validateMCPJSON('[]')).toBe('Root value must be a JSON object')
		expect(validateMCPJSON('{}')).toBe(
			'Missing required "mcpServers" property',
		)
		expect(validateMCPJSON('{"mcpServers": []}')).toBe(
			'"mcpServers" must be an object',
		)
		expect(validateMCPJSON('{"mcpServers": {"gw": "https://gw"}}')).toBe(
			'"gw" must be an object',
		)
		expect(validateMCPJSON('{"mcpServers": {')).toMatch(
			/^Invalid JSON syntax/,
		)
	})
})
