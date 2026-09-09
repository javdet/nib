-- "Index your FK columns", applied without checking whether a composite UNIQUE
-- already leads with that column. PostgreSQL serves a leftmost-prefix lookup
-- from the composite, so each of these is write amplification and nothing else.
--
-- chat_dialog_messages is the write-hot one -- a row per turn plus one per tool
-- call -- and it was maintaining two byte-identical B-trees on every insert.
--
-- Plain DROP INDEX, not CONCURRENTLY: the runner wraps each file in one
-- transaction, and CONCURRENTLY is illegal inside a transaction block. Each drop
-- takes a brief ACCESS EXCLUSIVE lock, which is acceptable for a drop.

-- covered by chat_dialog_messages_dialog_seq_unique (dialog_id, seq)
DROP INDEX IF EXISTS idx_chat_dialog_messages_dialog;
-- covered by kb_chunks_collection_source_chunk_unique (collection_id, ...)
DROP INDEX IF EXISTS idx_kb_chunks_collection_id;
-- covered by mcp_tools_server_name_unique (server_id, name)
DROP INDEX IF EXISTS idx_mcp_tools_server_id;
-- covered by tool_category_patterns_category_pattern_unique (category_id, pattern)
DROP INDEX IF EXISTS idx_tool_category_patterns_category_id;
-- covered by prompt_variables_scope_scope_name_name_unique (scope, ...)
DROP INDEX IF EXISTS idx_prompt_variables_scope;
-- covered by prompt_secrets_scope_scope_name_name_unique (scope, ...)
DROP INDEX IF EXISTS idx_prompt_secrets_scope;

-- Kept deliberately: idx_chat_attachments_message and
-- idx_mcp_tool_categories_category_id lead no composite UNIQUE.
