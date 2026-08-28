DROP INDEX IF EXISTS idx_mcp_tools_embedding_hnsw;

ALTER TABLE mcp_tools DROP COLUMN IF EXISTS embedding;
