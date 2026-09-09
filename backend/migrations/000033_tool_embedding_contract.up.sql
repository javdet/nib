-- Nothing recorded which model produced a stored mcp_tools.embedding: the
-- indexer holds embeddingModel and never writes it. Change llm.embeddingModel
-- and the column silently ends up holding two vector spaces at once -- no error,
-- just quietly worse results forever. kb_collections.embedding_model is the
-- pattern that got this right.
--
-- Nullable, because existing rows genuinely do not know which model made them.
-- The indexer treats a row whose model does not match the configured one as
-- stale and re-embeds it, so the column drains itself.

ALTER TABLE mcp_tools ADD COLUMN embedding_model text;
