CREATE INDEX IF NOT EXISTS idx_chat_dialog_messages_dialog ON chat_dialog_messages (dialog_id, seq);
CREATE INDEX IF NOT EXISTS idx_kb_chunks_collection_id ON kb_chunks (collection_id);
CREATE INDEX IF NOT EXISTS idx_mcp_tools_server_id ON mcp_tools (server_id);
CREATE INDEX IF NOT EXISTS idx_tool_category_patterns_category_id ON tool_category_patterns (category_id);
CREATE INDEX IF NOT EXISTS idx_prompt_variables_scope ON prompt_variables (scope);
CREATE INDEX IF NOT EXISTS idx_prompt_secrets_scope ON prompt_secrets (scope);
