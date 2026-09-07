import type { CatalogTool } from '../api/included-tools'
import { summarizeToolDescription } from './tool-description'

/** The skill that narrows each mode's included tools to a per-mode policy. */
export const DISTRIBUTE_TOOLS_SKILL = 'distribute-tools'

/**
 * One tool per line, bare name first.
 *
 * The name leads because that is what `update_included_tools` takes - the
 * server is context for the model, not part of the argument. The description is
 * clipped the same way the catalog tooltips clip it, so a large MCP surface
 * still fits in one message.
 */
function toolLine(tool: CatalogTool): string {
	const head = `- ${tool.name} (${tool.server})`
	const description = summarizeToolDescription(tool.description)
	return description ? `${head}: ${description}` : head
}

/**
 * The message that invokes the distribute-tools skill unambiguously.
 *
 * Skills are not slash commands - the text goes to the model verbatim, so
 * asking for the get_skill call by name takes the guess out of the invocation.
 * Only data travels in the message: the per-mode policy lives in the skill
 * markdown, which is seeded to the skills directory and editable by an
 * operator, so tuning it does not need a rebuild.
 */
export function buildDistributeToolsRequest(
	modes: string[],
	tools: CatalogTool[],
): string {
	return [
		`Call get_skill with name "${DISTRIBUTE_TOOLS_SKILL}" and follow its instructions exactly.`,
		'',
		'modes:',
		...modes.map((mode) => `- ${mode}`),
		'',
		'tools:',
		...tools.map(toolLine),
	].join('\n')
}
