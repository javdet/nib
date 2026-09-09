-- Order matters: tool_categories is referenced by tool_category_patterns and
-- mcp_tool_categories from 000019, so 000019.down has to run first or these
-- drops fail on the dependent foreign keys.
-- Roll back 000006_tool_catalog.up.sql.

DROP TABLE IF EXISTS mcp_tools;
DROP TABLE IF EXISTS mcp_server_categories;
DROP TABLE IF EXISTS mcp_servers;
DROP TABLE IF EXISTS tool_categories;
