import { summarizeToolDescription } from '@/features/included-tools/lib/tool-description'
import type {
	CategorizedTool,
	ToolCategory,
} from '../api/tool-categories'

/** The skill that sorts uncategorized tools into the existing tool categories. */
export const CATEGORIZE_TOOLS_SKILL = 'categorize-tools'

function categoryLine(category: ToolCategory): string {
	const description = category.description.trim()
	return description ? `- ${category.name}: ${description}` : `- ${category.name}`
}

/**
 * One tool per line, bare name first.
 *
 * The name leads because a category pattern is matched against the tool name
 * alone - the server is context for the model, not part of the pattern. The
 * description is clipped the same way the catalog tooltips clip it, so a large
 * MCP surface still fits in one message.
 */
function toolLine(tool: CategorizedTool): string {
	const head = `- ${tool.name} (${tool.server})`
	const description = summarizeToolDescription(tool.description)
	return description ? `${head}: ${description}` : head
}

/**
 * The message that invokes the categorize-tools skill unambiguously.
 *
 * Skills are not slash commands - the text goes to the model verbatim, so
 * asking for the get_skill call by name takes the guess out of the invocation.
 * Both lists the skill requires travel in the message: it has no tool that can
 * read the category list or the uncategorized bucket for itself.
 */
export function buildCategorizeToolsRequest(
	categories: ToolCategory[],
	tools: CategorizedTool[],
): string {
	return [
		`Call get_skill with name "${CATEGORIZE_TOOLS_SKILL}" and follow its instructions exactly.`,
		'',
		'categories:',
		...categories.map(categoryLine),
		'',
		'tools:',
		...tools.map(toolLine),
	].join('\n')
}
